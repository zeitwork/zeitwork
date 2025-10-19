package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v67/github"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// TokenService handles GitHub App authentication and token generation
type TokenService struct {
	appID      int64
	privateKey *rsa.PrivateKey
}

// NewTokenService creates a new GitHub token service
// privateKeyBase64 should be a base64-encoded PEM private key
func NewTokenService(appID string, privateKeyBase64 string) (*TokenService, error) {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub App ID: %w", err)
	}

	// Decode base64 to get PEM
	privateKeyPEM, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 private key: %w", err)
	}

	// Parse the PEM-encoded private key
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the private key")
	}

	// Parse the RSA private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format as fallback
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key (PKCS1: %v, PKCS8: %v)", err, err2)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
	}

	return &TokenService{
		appID:      id,
		privateKey: privateKey,
	}, nil
}

// createJWT generates a JWT for GitHub App authentication
func (s *TokenService) createJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    strconv.FormatInt(s.appID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// GetInstallationToken generates a short-lived access token for the given GitHub installation
func (s *TokenService) GetInstallationToken(ctx context.Context, db *database.DB, installationUUID string) (string, error) {
	// Parse the UUID
	installationID, err := uuid.Parse(installationUUID)
	if err != nil {
		return "", fmt.Errorf("invalid installation UUID: %w", err)
	}

	// Look up the GitHub installation ID from our database
	installation, err := db.Queries().GetGithubInstallationByID(ctx, installationID)
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub installation: %w", err)
	}

	// Create a JWT for GitHub App authentication
	jwtToken, err := s.createJWT()
	if err != nil {
		return "", fmt.Errorf("failed to create JWT: %w", err)
	}

	// Create a GitHub client authenticated as the app
	client := github.NewClient(&http.Client{
		Transport: &jwtTransport{
			token: jwtToken,
		},
	})

	// Create an installation access token
	token, _, err := client.Apps.CreateInstallationToken(
		ctx,
		int64(installation.GithubInstallationID),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create installation token: %w", err)
	}

	return token.GetToken(), nil
}

// jwtTransport is a custom http.RoundTripper that adds JWT authentication
type jwtTransport struct {
	token string
}

func (t *jwtTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	return http.DefaultTransport.RoundTrip(req)
}
