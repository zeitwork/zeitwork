package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

type GitHubWebhookPayload struct {
	Action       string             `json:"action"`
	Repository   GitHubRepository   `json:"repository"`
	Ref          string             `json:"ref"`   // For push events
	After        string             `json:"after"` // Commit hash for push events
	PullRequest  GitHubPullRequest  `json:"pull_request,omitempty"`
	Sender       GitHubSender       `json:"sender"`
	Installation GitHubInstallation `json:"installation,omitempty"`
}

type GitHubRepository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Owner    struct {
		Login string `json:"login"`
		ID    int    `json:"id"`
	} `json:"owner"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
}

type GitHubPullRequest struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Head   struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Merged bool `json:"merged"`
}

type GitHubSender struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

type GitHubInstallation struct {
	ID      int `json:"id"`
	Account struct {
		Login string `json:"login"`
		ID    int    `json:"id"`
	} `json:"account"`
}

// handleGitHubWebhook handles GitHub webhook events
func (s *Service) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify webhook signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, "Missing signature", http.StatusUnauthorized)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature
	if !s.verifyGitHubSignature(body, signature) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Get event type
	eventType := r.Header.Get("X-GitHub-Event")

	// Parse payload
	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.Error("Failed to parse webhook payload", "error", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Handle different event types
	switch eventType {
	case "push":
		if err := s.handlePushEvent(ctx, &payload); err != nil {
			s.logger.Error("Failed to handle push event", "error", err)
			http.Error(w, "Failed to process push event", http.StatusInternalServerError)
			return
		}

	case "pull_request":
		if err := s.handlePullRequestEvent(ctx, &payload); err != nil {
			s.logger.Error("Failed to handle pull request event", "error", err)
			http.Error(w, "Failed to process pull request event", http.StatusInternalServerError)
			return
		}

	case "installation":
		if err := s.handleInstallationEvent(ctx, &payload); err != nil {
			s.logger.Error("Failed to handle installation event", "error", err)
			http.Error(w, "Failed to process installation event", http.StatusInternalServerError)
			return
		}

	default:
		s.logger.Info("Ignoring webhook event", "type", eventType)
	}

	// Respond with success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// verifyGitHubSignature verifies the GitHub webhook signature
func (s *Service) verifyGitHubSignature(payload []byte, signature string) bool {
	// Remove "sha256=" prefix
	signature = strings.TrimPrefix(signature, "sha256=")

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(s.config.GitHubSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// handlePushEvent handles GitHub push events
func (s *Service) handlePushEvent(ctx context.Context, payload *GitHubWebhookPayload) error {
	// Extract branch name from ref
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	// Find projects connected to this repository
	repoFullName := payload.Repository.FullName
	// TODO: Fix this when github_repo field is added to projects table
	// For now, return empty set
	projects, err := s.db.Queries().ProjectFindByGitHubRepo(ctx)
	_ = repoFullName // Suppress unused variable warning
	if err != nil {
		return fmt.Errorf("failed to find projects: %w", err)
	}

	// Create deployments for each project
	for _, project := range projects {
		// Skip if not default branch (unless configured otherwise)
		if branch != payload.Repository.DefaultBranch && branch != "main" && branch != "master" {
			s.logger.Info("Skipping non-default branch",
				"branch", branch,
				"default", payload.Repository.DefaultBranch,
				"project", project.ID)
			continue
		}

		// Generate nano ID for deployment
		nanoID := generateNanoID(7)

		// Get organization for deployment URL
		org, err := s.db.Queries().OrganisationFindById(ctx, project.OrganisationID)
		if err != nil {
			s.logger.Error("Failed to get organization", "error", err)
			continue
		}

		deploymentURL := fmt.Sprintf("%s-%s-%s.zeitwork.app", project.Slug, nanoID, org.Slug)

		// Get production environment
		envs, err := s.db.Queries().ProjectEnvironmentFindByProject(ctx, project.ID)
		if err != nil {
			s.logger.Error("Failed to get environments", "error", err)
			continue
		}

		var envID pgtype.UUID
		for _, env := range envs {
			if env.Name == "production" {
				envID = env.ID
				break
			}
		}

		if !envID.Valid {
			// Create production environment
			env, err := s.db.Queries().ProjectEnvironmentCreate(ctx, &database.ProjectEnvironmentCreateParams{
				ProjectID:      project.ID,
				Name:           "production",
				OrganisationID: project.OrganisationID,
			})
			if err != nil {
				s.logger.Error("Failed to create environment", "error", err)
				continue
			}
			envID = env.ID
		}

		// Create image record
		imageParams := database.ImageCreateParams{
			Name:   fmt.Sprintf("%s:%s", project.Slug, payload.After[:7]),
			Status: "pending",
			Repository: json.RawMessage(fmt.Sprintf(`{"type":"github","repo":"%s","commit":"%s","branch":"%s"}`,
				repoFullName, payload.After, branch)),
			ImageHash: "", // Will be set after build
		}

		image, err := s.db.Queries().ImageCreate(ctx, &imageParams)
		if err != nil {
			s.logger.Error("Failed to create image", "error", err)
			continue
		}

		// Create deployment
		deploymentParams := database.DeploymentCreateParams{
			ProjectID:            project.ID,
			ProjectEnvironmentID: envID,
			Status:               "pending",
			CommitHash:           payload.After,
			ImageID:              pgtype.UUID{Bytes: image.ID.Bytes, Valid: true},
			OrganisationID:       project.OrganisationID,
			DeploymentUrl:        pgtype.Text{String: deploymentURL, Valid: true},
			Nanoid:               pgtype.Text{String: nanoID, Valid: true},
			MinInstances:         3,
			RolloutStrategy:      "blue-green",
		}

		deployment, err := s.db.Queries().DeploymentCreate(ctx, &deploymentParams)
		if err != nil {
			s.logger.Error("Failed to create deployment", "error", err)
			continue
		}

		// Add to build queue
		buildParams := database.BuildQueueCreateParams{
			ProjectID:  pgtype.UUID{Bytes: project.ID.Bytes, Valid: true},
			ImageID:    image.ID,
			Priority:   0,
			Status:     "pending",
			GithubRepo: repoFullName,
			CommitHash: payload.After,
			Branch:     branch,
		}

		_, err = s.db.Queries().BuildQueueCreate(ctx, &buildParams)
		if err != nil {
			s.logger.Error("Failed to add to build queue", "error", err)
		}

		s.logger.Info("Created deployment from push",
			"project", project.ID,
			"deployment", deployment.ID,
			"commit", payload.After[:7],
			"branch", branch)
	}

	return nil
}

// handlePullRequestEvent handles GitHub pull request events
func (s *Service) handlePullRequestEvent(ctx context.Context, payload *GitHubWebhookPayload) error {
	// Only handle opened and synchronize actions for preview deployments
	if payload.Action != "opened" && payload.Action != "synchronize" {
		return nil
	}

	// Find projects connected to this repository
	repoFullName := payload.Repository.FullName
	// TODO: Fix this when github_repo field is added to projects table
	// For now, return empty set
	projects, err := s.db.Queries().ProjectFindByGitHubRepo(ctx)
	_ = repoFullName // Suppress unused variable warning
	if err != nil {
		return fmt.Errorf("failed to find projects: %w", err)
	}

	// Create preview deployments
	for _, project := range projects {
		// Generate nano ID for preview deployment
		nanoID := fmt.Sprintf("pr%d-%s", payload.PullRequest.Number, generateNanoID(5))

		// Get organization
		org, err := s.db.Queries().OrganisationFindById(ctx, project.OrganisationID)
		if err != nil {
			s.logger.Error("Failed to get organization", "error", err)
			continue
		}

		deploymentURL := fmt.Sprintf("%s-%s-%s.zeitwork.app", project.Slug, nanoID, org.Slug)

		// Get or create preview environment
		envName := fmt.Sprintf("pr-%d", payload.PullRequest.Number)
		envs, err := s.db.Queries().ProjectEnvironmentFindByProject(ctx, project.ID)
		if err != nil {
			s.logger.Error("Failed to get environments", "error", err)
			continue
		}

		var envID pgtype.UUID
		for _, env := range envs {
			if env.Name == envName {
				envID = env.ID
				break
			}
		}

		if !envID.Valid {
			// Create preview environment
			env, err := s.db.Queries().ProjectEnvironmentCreate(ctx, &database.ProjectEnvironmentCreateParams{
				ProjectID:      project.ID,
				Name:           envName,
				OrganisationID: project.OrganisationID,
			})
			if err != nil {
				s.logger.Error("Failed to create environment", "error", err)
				continue
			}
			envID = env.ID
		}

		// Create image record
		imageParams := database.ImageCreateParams{
			Name:   fmt.Sprintf("%s:pr-%d-%s", project.Slug, payload.PullRequest.Number, payload.PullRequest.Head.SHA[:7]),
			Status: "pending",
			Repository: json.RawMessage(fmt.Sprintf(`{"type":"github","repo":"%s","commit":"%s","branch":"%s","pr":%d}`,
				repoFullName, payload.PullRequest.Head.SHA, payload.PullRequest.Head.Ref, payload.PullRequest.Number)),
			ImageHash: "",
		}

		image, err := s.db.Queries().ImageCreate(ctx, &imageParams)
		if err != nil {
			s.logger.Error("Failed to create image", "error", err)
			continue
		}

		// Create deployment
		deploymentParams := database.DeploymentCreateParams{
			ProjectID:            project.ID,
			ProjectEnvironmentID: envID,
			Status:               "pending",
			CommitHash:           payload.PullRequest.Head.SHA,
			ImageID:              pgtype.UUID{Bytes: image.ID.Bytes, Valid: true},
			OrganisationID:       project.OrganisationID,
			DeploymentUrl:        pgtype.Text{String: deploymentURL, Valid: true},
			Nanoid:               pgtype.Text{String: nanoID, Valid: true},
			MinInstances:         1, // Less instances for preview
			RolloutStrategy:      "blue-green",
		}

		deployment, err := s.db.Queries().DeploymentCreate(ctx, &deploymentParams)
		if err != nil {
			s.logger.Error("Failed to create deployment", "error", err)
			continue
		}

		// Add to build queue with higher priority
		buildParams := database.BuildQueueCreateParams{
			ProjectID:  pgtype.UUID{Bytes: project.ID.Bytes, Valid: true},
			ImageID:    image.ID,
			Priority:   10, // Higher priority for PR builds
			Status:     "pending",
			GithubRepo: repoFullName,
			CommitHash: payload.PullRequest.Head.SHA,
			Branch:     payload.PullRequest.Head.Ref,
		}

		_, err = s.db.Queries().BuildQueueCreate(ctx, &buildParams)
		if err != nil {
			s.logger.Error("Failed to add to build queue", "error", err)
		}

		s.logger.Info("Created preview deployment from PR",
			"project", project.ID,
			"deployment", deployment.ID,
			"pr", payload.PullRequest.Number,
			"commit", payload.PullRequest.Head.SHA[:7])
	}

	return nil
}

// handleInstallationEvent handles GitHub app installation events
func (s *Service) handleInstallationEvent(ctx context.Context, payload *GitHubWebhookPayload) error {
	if payload.Action != "created" {
		return nil
	}

	// Store installation information
	params := database.GithubInstallationCreateParams{
		GithubInstallationID: int32(payload.Installation.ID),
		GithubOrgID:          int32(payload.Installation.Account.ID),
	}

	_, err := s.db.Queries().GithubInstallationCreate(ctx, &params)
	if err != nil {
		return fmt.Errorf("failed to store installation: %w", err)
	}

	s.logger.Info("GitHub app installed",
		"installation_id", payload.Installation.ID,
		"account", payload.Installation.Account.Login)

	return nil
}
