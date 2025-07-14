package api

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/zeitwork/zeitwork/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type CreateAppRequest struct {
	Name           string `json:"name"`
	OrganisationID int32  `json:"organisationId"`
	GithubOwner    string `json:"githubOwner"`
	GithubRepo     string `json:"githubRepo"`
}

func (a *API) CreateApp(c echo.Context) error {
	var req CreateAppRequest
	if err := c.Bind(&req); err != nil {
		return errors.New("invalid request payload")
	}

	// find the organisation by ID
	organisation, err := a.Services.DB.OrganisationFindByID(c.Request().Context(), req.OrganisationID)
	if err != nil {
		return errors.New("organisation not found")
	}

	// if the org does not have an installation yet, skip
	if !organisation.InstallationID.Valid {
		return errors.New("installation doesn't already exists")
	}

	// use the organisation to get a github instance
	gclient, err := a.Services.Github.GetClientForInstallation(organisation.InstallationID.Int64)
	if err != nil {
		return err
	}

	// we can fetch the Repo
	repo, _, err := gclient.Repositories.Get(c.Request().Context(), req.GithubOwner, req.GithubRepo)
	if err != nil {
		return errors.New("failed to fetch GitHub repository")
	}

	// download the latest commit SHA
	branch, _, err := gclient.Repositories.GetBranch(c.Request().Context(), req.GithubOwner, req.GithubRepo, repo.GetDefaultBranch(), 5)
	if err != nil {
		return errors.New("failed to fetch latest commit SHA")
	}
	if branch == nil || branch.Commit == nil || branch.Commit.GetSHA() == "" {
		return errors.New("failed to fetch latest commit SHA: branch or commit is nil")
	}

	// ensure a namespace in k8s exists for the organisation
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("caas-%s", organisation.Slug)}}
	_, err = controllerutil.CreateOrUpdate(c.Request().Context(), a.Services.K8s, ns, func() error {
		ns.Labels = map[string]string{
			"zeitwork.com/organisation-id": fmt.Sprintf("%d", organisation.ID),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update namespace: %w", err)
	}

	// now that a namespace exists, ensure the app is created in the namespace
	app := v1alpha1.App{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("repo-%d", *repo.ID), Namespace: ns.Name}}
	_, err = controllerutil.CreateOrUpdate(c.Request().Context(), a.Services.K8s, &app, func() error {
		app.ObjectMeta.Labels = map[string]string{
			"zeitwork.com/organisation-id": fmt.Sprintf("%d", organisation.ID),
		}
		app.Spec = v1alpha1.AppSpec{
			Description:        req.Name,
			DesiredRevisionSHA: pointer.String(branch.Commit.GetSHA()),
			FQDN:               nil,
			GithubOwner:        req.GithubOwner,
			GithubRepo:         req.GithubRepo,
			GithubInstallation: organisation.InstallationID.Int64,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update app: %w", err)
	}

	fmt.Printf("Webhook created for repository %s/%s\n", req.GithubOwner, req.GithubRepo)
	return c.JSON(201, map[string]string{
		"message": "App created successfully",
	})
}
