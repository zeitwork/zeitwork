package services

import (
	"context"
	"fmt"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v73/github"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"net/http"
	"os"
)

const appID = 1593721

var PrivateKeyName = os.Getenv("GITHUB_PRIVATE_KEY_NAME")
var ClientID = os.Getenv("GITHUB_CLIENT_ID")
var ClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")

type Github struct {
	Client *github.Client
}

func NewGithub() (Github, error) {
	itr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, appID, PrivateKeyName)
	if err != nil {
		return Github{}, err
	}

	client := github.NewClient(&http.Client{Transport: itr})

	return Github{
		Client: client,
	}, nil
}

func (g *Github) GetClientForInstallation(installID int64) (*github.Client, error) {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installID, PrivateKeyName)
	if err != nil {
		return nil, err
	}

	return github.NewClient(&http.Client{Transport: itr}), nil
}

func (g *Github) GetTokenForInstallation(ctx context.Context, installID int64) (string, error) {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installID, PrivateKeyName)
	if err != nil {
		return "", err
	}

	token, err := itr.Token(ctx)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (g *Github) GetInstallationURL() string {
	return fmt.Sprintf("https://github.com/apps/%s/installations/new", os.Getenv("GITHUB_APP_NAME"))
}

func (g *Github) ExchangeCodeForUser(ctx context.Context, code string) (github.User, error) {
	// Configure OAuth2
	conf := &oauth2.Config{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		Endpoint:     oauthGithub.Endpoint,
		Scopes:       []string{"user:email"},
	}

	// Exchange code for token
	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return github.User{}, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Create HTTP client with token
	client := conf.Client(ctx, token)

	// Create GitHub client
	githubClient := github.NewClient(client)

	// Get user information
	user, _, err := githubClient.Users.Get(ctx, "")
	if err != nil {
		return github.User{}, fmt.Errorf("failed to get user info: %w", err)
	}

	if user == nil || user.Login == nil {
		return github.User{}, fmt.Errorf("user login is nil")
	}

	return *user, nil
}
