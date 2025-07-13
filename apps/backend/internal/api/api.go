package api

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/zeitwork/zeitwork/api/v1alpha1"
	"github.com/zeitwork/zeitwork/internal/services"
	"github.com/zeitwork/zeitwork/internal/services/db"
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

	e.GET("/", func(c echo.Context) error {
		return c.HTML(200, fmt.Sprintf(`
Welcome to CAAS API!
Login to github now to get started: <a href='%s'>Login with GitHub</a>'
`, svc.Github.GetInstallationURL()))
	})

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

			// if this is a new installation, insert into the db
			if event.Action == "created" {
				_, err := svc.DB.OrganisationInsert(c.Request().Context(), db.OrganisationInsertParams{
					InstallationID: event.Installation.ID,
					GithubUsername: event.Installation.Account.Login,
				})
				if err != nil {
					return err
				}
			}
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
			gclient, err := svc.Github.GetClientForInstallation(organisation.InstallationID)
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
			err = svc.K8s.Get(c.Request().Context(), client.ObjectKey{Namespace: fmt.Sprintf("caas-%s", organisation.GithubUsername), Name: fmt.Sprintf("repo-%d", *repo.ID)}, &app)
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
