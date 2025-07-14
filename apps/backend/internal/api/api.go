package api

import (
	"errors"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/zeitwork/zeitwork/api/v1alpha1"
	"github.com/zeitwork/zeitwork/internal/auth"
	"github.com/zeitwork/zeitwork/internal/graph"
	"github.com/zeitwork/zeitwork/internal/services"
	"io"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type API struct {
	Services *services.Services
}

func StartAPI(svc *services.Services) {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{}))

	// initialize graphql api
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{
		Services: svc,
	}}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](100)})

	e.GET("/", func(c echo.Context) error {
		return c.HTML(200, fmt.Sprintf(`
Welcome to CAAS API!
Login to github now to get started: <a href='%s'>Login with GitHub</a>'
`, svc.Github.GetInstallationURL()))
	})

	e.Any("/playground", echo.WrapHandler(playground.AltairHandler("Playground", "/graph", nil)))
	e.Any("/graph", echo.WrapHandler(srv), auth.JWTMiddleware())

	e.GET("/auth/github/callback", func(c echo.Context) error {
		return c.String(200, "GitHub authentication callback received")
	})

	e.POST("/webhook/github", func(c echo.Context) error {
		fmt.Printf("Received GitHub webhook event: %s\n", c.Request().Header.Get("X-GitHub-Event"))

		switch c.Request().Header.Get("X-GitHub-Event") {
		case "installation":
			var event GithubEventInstallation
			if err := c.Bind(&event); err != nil {
				return c.String(400, "Invalid installation event payload")
			}
			fmt.Printf("Installation event: %s for installation ID %d by user %s\n",
				event.Action, event.Installation.ID, event.Installation.Account.Login)
		case "push":
			var event GithubEventPush
			if err := c.Bind(&event); err != nil {
				return c.String(400, "Invalid push event payload")
			}

			// step 1: find the organisation by the installation ID
			organisation, err := svc.DB.OrganisationFindByInstallationID(c.Request().Context(), event.Installation.ID)
			if err != nil {
				return c.String(200, "Organisation not found for installation ID")
			}

			// step 2: get the GitHub client for the organisation and fetch the repository to make sure this is a valid push event
			gclient, err := svc.Github.GetClientForInstallation(organisation.InstallationID.Int64)
			if err != nil {
				return c.String(500, "Failed to get GitHub client for installation")
			}

			repo, _, err := gclient.Repositories.Get(c.Request().Context(), event.Repository.Owner.Login, event.Repository.Name)
			if err != nil {
				return c.String(500, "Failed to fetch GitHub repository")
			}

			// download the latest commit SHA
			branch, _, err := gclient.Repositories.GetBranch(c.Request().Context(), event.Repository.Owner.Login, event.Repository.Name, repo.GetDefaultBranch(), 5)
			if err != nil {
				return errors.New("failed to fetch latest commit SHA")
			}
			if branch == nil || branch.Commit == nil || branch.Commit.GetSHA() == "" {
				return errors.New("failed to fetch latest commit SHA: branch or commit is nil")
			}

			// all checks out so far, update the App given it exists
			var app v1alpha1.App
			err = svc.K8s.Get(c.Request().Context(), client.ObjectKey{Namespace: fmt.Sprintf("caas-%s", organisation.Slug), Name: fmt.Sprintf("repo-%d", *repo.ID)}, &app)
			if err != nil {
				// doesn't exist, don't care.
				return c.String(200, "App not found for repository, ignoring push event")
			}

			// update the app with the latest commit SHA
			app.Spec.DesiredRevisionSHA = pointer.String(branch.Commit.GetSHA())
			err = svc.K8s.Update(c.Request().Context(), &app)
			if err != nil {
				return err
			}

			// log the push event
			fmt.Printf("Push event: %s to repository %s/%s with latest commit SHA %s\n",
				event.Ref, event.Repository.Owner.Login, event.Repository.Name, branch.Commit.GetSHA())

		default:
			body, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return c.String(500, "Failed to read request body")
			}
			defer c.Request().Body.Close()
			fmt.Printf("Request body: %s\n", body)
		}

		// Handle GitHub webhook events here
		// You can use svc.Github to interact with the GitHub API
		return c.String(200, "GitHub webhook event received")
	})

	api := API{Services: svc}

	e.POST("/app/setup", api.CreateApp)

	panic(e.Start(":8080"))
}

type GithubEventInstallation struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		}
	}
}

type GithubEventPush struct {
	Ref          string `json:"ref"`
	After        string `json:"after"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		}
	} `json:"repository"`
}
