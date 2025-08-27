# GitHub Integration Implementation

## Overview

Successfully implemented GitHub repository integration for the Zeitwork platform, enabling automatic deployments from GitHub repositories.

## Database Schema Changes

### Projects Table

Added three new fields to the `projects` table:

```sql
ALTER TABLE "projects" ADD COLUMN "github_repo" text;
ALTER TABLE "projects" ADD COLUMN "github_installation_id" integer;
ALTER TABLE "projects" ADD COLUMN "github_default_branch" text DEFAULT 'main';
```

- **github_repo**: Stores the repository in "owner/repo" format
- **github_installation_id**: References the GitHub App installation
- **github_default_branch**: Default branch for deployments (defaults to "main")

### Migration Created

- Migration file: `migrations/0002_icy_kat_farrell.sql`
- Includes foreign key constraint to `github_installations` table

## SQL Query Updates

### Projects Queries

Updated `/internal/database/queries/projects.sql`:

1. **ProjectCreate**: Now accepts GitHub fields

```sql
INSERT INTO projects (
    name, slug, organisation_id,
    github_repo, github_installation_id, github_default_branch
) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;
```

2. **ProjectUpdate**: Includes GitHub field updates

```sql
UPDATE projects SET
    name = $2,
    slug = $3,
    github_repo = $4,
    github_installation_id = $5,
    github_default_branch = $6,
    updated_at = NOW()
WHERE id = $1 RETURNING *;
```

3. **ProjectFindByGitHubRepo**: Fixed to use actual fields

```sql
SELECT * FROM projects
WHERE github_repo = $1
    AND github_installation_id = $2
ORDER BY created_at DESC;
```

### Routing Cache Queries

Created `/internal/database/queries/routing.sql` with:

- RoutingCacheUpsert
- RoutingCacheFindByDomain
- RoutingCacheFindByDeployment
- RoutingCacheDeleteByDomain
- RoutingCacheDeleteByDeployment
- RoutingCacheCleanup

## API Updates

### Project API Enhancements

Updated `/internal/api/projects.go`:

1. **Request/Response Types**:

```go
type CreateProjectRequest struct {
    Name                 string  `json:"name"`
    Slug                 string  `json:"slug"`
    OrganizationID       string  `json:"organization_id"`
    GitHubRepo           *string `json:"github_repo,omitempty"`
    GitHubInstallationID *int32  `json:"github_installation_id,omitempty"`
    GitHubDefaultBranch  *string `json:"github_default_branch,omitempty"`
}
```

2. **Project Response**: Includes GitHub fields in API responses
3. **Create/Update Handlers**: Process GitHub integration fields

### Webhook Processing

Fixed `/internal/api/webhooks.go`:

1. **Push Event Handler**:

   - Now properly queries projects by repository and installation ID
   - Creates deployments for matching projects on default branch pushes

2. **Pull Request Handler**:
   - Creates preview deployments for PRs
   - Uses GitHub installation ID for authentication

## Usage Examples

### Creating a Project with GitHub Integration

```bash
curl -X POST https://api.zeitwork.com/api/v1/projects \
  -H "Authorization: Bearer TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My App",
    "slug": "my-app",
    "organization_id": "org-uuid",
    "github_repo": "myorg/myapp",
    "github_installation_id": 12345,
    "github_default_branch": "main"
  }'
```

### GitHub Webhook Flow

1. GitHub sends webhook to `/api/webhooks/github`
2. System validates webhook signature
3. Finds projects matching repository and installation
4. Creates deployment for push to default branch
5. Creates preview deployment for pull requests

## Benefits

1. **Automatic Deployments**: Push to main branch triggers deployment
2. **Preview Environments**: Pull requests get preview deployments
3. **Multi-Project Support**: One repository can deploy to multiple projects
4. **Security**: Uses GitHub App installation for authentication
5. **Branch Control**: Respects configured default branch

## Testing

To test the integration:

1. Create a GitHub App and install it on your repository
2. Configure webhook URL: `https://your-domain/api/webhooks/github`
3. Create a project with GitHub integration fields
4. Push to your default branch
5. Verify deployment is created automatically

## Next Steps

- Implement build queue processing for created deployments
- Add support for deployment status updates back to GitHub
- Implement GitHub commit status API integration
- Add support for deployment environments (staging, production)
- Implement automatic rollback on failure
