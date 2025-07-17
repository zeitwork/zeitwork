package services

import (
	"context"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v73/github"
	"net/http"
	"os"
)

const appID = 1616586

var PrivateKeyName = os.Getenv("GITHUB_PRIVATE_KEY_NAME")

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
