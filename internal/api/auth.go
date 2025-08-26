package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

type GitHubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type GitHubAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

type AuthResponse struct {
	Token   string `json:"token"`
	User    *User  `json:"user"`
	Message string `json:"message,omitempty"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// handleGitHubAuth initiates GitHub OAuth flow
func (s *Service) handleGitHubAuth(w http.ResponseWriter, r *http.Request) {
	// Generate state for CSRF protection
	state := generateState()

	// Store state in cookie (in production, use a proper session store)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	// Build GitHub OAuth URL
	params := url.Values{}
	params.Add("client_id", s.config.GitHubClientID)
	params.Add("redirect_uri", s.config.BaseURL+"/v1/auth/github/callback")
	params.Add("scope", "read:user user:email")
	params.Add("state", state)

	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?%s", params.Encode())

	// Redirect to GitHub
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleGitHubCallback handles the GitHub OAuth callback
func (s *Service) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify state
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value != state {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for access token
	token, err := s.exchangeCodeForToken(code)
	if err != nil {
		s.logger.Error("Failed to exchange code for token", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Get user info from GitHub
	githubUser, err := s.getGitHubUser(token)
	if err != nil {
		s.logger.Error("Failed to get GitHub user", "error", err)
		http.Error(w, "Failed to get user information", http.StatusInternalServerError)
		return
	}

	// Create or update user in database
	user, err := s.createOrUpdateUser(ctx, githubUser)
	if err != nil {
		s.logger.Error("Failed to create/update user", "error", err)
		http.Error(w, "Failed to process user", http.StatusInternalServerError)
		return
	}

	// Create JWT token
	jwtToken, err := s.createJWTToken(user.ID)
	if err != nil {
		s.logger.Error("Failed to create JWT token", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Create session in database
	sessionToken := generateSessionToken()
	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days

	params := database.SessionCreateParams{
		UserID:    user.ID,
		Token:     sessionToken,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}

	_, err = s.db.Queries().SessionCreate(ctx, &params)
	if err != nil {
		s.logger.Error("Failed to create session", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Return token and user info
	response := AuthResponse{
		Token: jwtToken,
		User: &User{
			ID:       uuid.UUID(user.ID.Bytes).String(),
			Name:     user.Name,
			Email:    user.Email,
			Username: user.Username,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleLogout logs out the user
func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 {
		http.Error(w, "Missing authorization header", http.StatusBadRequest)
		return
	}

	token := authHeader[7:] // Remove "Bearer " prefix

	// Delete session from database
	err := s.db.Queries().SessionDeleteByToken(ctx, token)
	if err != nil && err != pgx.ErrNoRows {
		s.logger.Error("Failed to delete session", "error", err)
		http.Error(w, "Failed to logout", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// exchangeCodeForToken exchanges GitHub authorization code for access token
func (s *Service) exchangeCodeForToken(code string) (string, error) {
	// Prepare request
	params := url.Values{}
	params.Add("client_id", s.config.GitHubClientID)
	params.Add("client_secret", s.config.GitHubSecret)
	params.Add("code", code)

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.URL.RawQuery = params.Encode()

	// Make request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse response
	var tokenResp GitHubAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

// getGitHubUser gets user information from GitHub
func (s *Service) getGitHubUser(token string) (*GitHubUser, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

// createOrUpdateUser creates or updates a user in the database
func (s *Service) createOrUpdateUser(ctx context.Context, githubUser *GitHubUser) (*database.User, error) {
	// Check if user exists
	user, err := s.db.Queries().UserFindByEmail(ctx, githubUser.Email)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if err == pgx.ErrNoRows {
		// Create new user
		params := database.UserCreateParams{
			Name:         githubUser.Name,
			Email:        githubUser.Email,
			Username:     githubUser.Login,
			GithubUserID: pgtype.Int4{Int32: int32(githubUser.ID), Valid: true},
		}
		user, err = s.db.Queries().UserCreate(ctx, &params)
		if err != nil {
			return nil, err
		}
	} else {
		// Update existing user
		params := database.UserUpdateParams{
			ID:           user.ID,
			Name:         githubUser.Name,
			Username:     githubUser.Login,
			GithubUserID: pgtype.Int4{Int32: int32(githubUser.ID), Valid: true},
		}
		user, err = s.db.Queries().UserUpdate(ctx, &params)
		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

// createJWTToken creates a JWT token for the user
func (s *Service) createJWTToken(userID pgtype.UUID) (string, error) {
	claims := jwt.MapClaims{
		"user_id": uuid.UUID(userID.Bytes).String(),
		"exp":     time.Now().Add(30 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

// validateToken validates a JWT token
func (s *Service) validateToken(ctx context.Context, tokenString string) (pgtype.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return pgtype.UUID{}, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid token")
	}

	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		return pgtype.UUID{}, fmt.Errorf("user_id not found in token")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return pgtype.UUID{}, err
	}

	// Verify session exists
	session, err := s.db.Queries().SessionFindByUserAndNotExpired(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("session not found or expired")
	}

	return session.UserID, nil
}

// generateState generates a random state for CSRF protection
func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// generateSessionToken generates a random session token
func generateSessionToken() string {
	b := make([]byte, 64)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
