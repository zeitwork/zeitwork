import { App, Octokit, RequestError } from "octokit";

// Main composable for GitHub API interactions
export function useGitHub() {
  const config = useRuntimeConfig();

  // Decode base64-encoded private key
  const privateKey = Buffer.from(config.githubAppPrivateKey, "base64").toString("utf-8");

  // GitHub App instance
  const app = new App({
    appId: config.githubAppId,
    privateKey: privateKey,
  });

  // Get an Octokit instance for a specific installation
  async function getInstallationOctokit(installationId: number) {
    try {
      const octokit = await app.getInstallationOctokit(installationId);
      return { data: octokit, error: null };
    } catch (error) {
      if (error instanceof RequestError) {
        return {
          data: null,
          error: new Error(
            `Failed to get installation Octokit (${error.status}): ${error.message}`,
          ),
        };
      }
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Get an installation access token
  async function getInstallationToken(installationId: number) {
    try {
      const { data } = await app.octokit.rest.apps.createInstallationAccessToken({
        installation_id: installationId,
      });
      return { data: data.token, error: null };
    } catch (error) {
      if (error instanceof RequestError) {
        // Handle specific GitHub API errors
        switch (error.status) {
          case 404:
            return {
              data: null,
              error: new Error(`Installation ${installationId} not found`),
            };
          case 403:
            return {
              data: null,
              error: new Error(`Access denied for installation ${installationId}`),
            };
          case 401:
            return {
              data: null,
              error: new Error(`Authentication failed - check GitHub App credentials`),
            };
        }
      }
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Fetch repository information
  async function getRepository(installationId: number, owner: string, repo: string) {
    const { data: octokit, error: octokitError } = await getInstallationOctokit(installationId);
    if (octokitError) return { data: null, error: octokitError };

    try {
      const { data } = await octokit.rest.repos.get({
        owner,
        repo,
      });

      return {
        data: {
          id: data.id,
          defaultBranch: data.default_branch,
          fullName: data.full_name,
        },
        error: null,
      };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Get the latest commit SHA from a branch
  async function getLatestCommitSHA(
    installationId: number,
    owner: string,
    repo: string,
    branch: string,
  ) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.getBranch({
        owner,
        repo,
        branch,
      });

      return { data: data.commit.sha, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Get commit information
  async function getCommit(installationId: number, owner: string, repo: string, ref: string) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.getCommit({
        owner,
        repo,
        ref,
      });

      return { data, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // List branches for a repository
  async function listBranches(installationId: number, owner: string, repo: string) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.listBranches({
        owner,
        repo,
        per_page: 100, // Reasonable default
      });

      return { data, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Trigger a repository dispatch event (useful for CI/CD)
  async function dispatchRepositoryEvent(
    installationId: number,
    owner: string,
    repo: string,
    eventType: string,
    clientPayload?: Record<string, any>,
  ) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      await octokitResult.data.rest.repos.createDispatchEvent({
        owner,
        repo,
        event_type: eventType,
        client_payload: clientPayload || {},
      });

      return { data: undefined, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // List repository collaborators
  async function listCollaborators(installationId: number, owner: string, repo: string) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.listCollaborators({
        owner,
        repo,
      });

      return { data, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Create a deployment
  async function createDeployment(
    installationId: number,
    owner: string,
    repo: string,
    ref: string,
    environment: string = "production",
    description?: string,
  ) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.createDeployment({
        owner,
        repo,
        ref,
        environment,
        description,
        auto_merge: false,
        required_contexts: [], // Skip status checks
      });

      return { data, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Update deployment status
  async function createDeploymentStatus(
    installationId: number,
    owner: string,
    repo: string,
    deploymentId: number,
    state: "error" | "failure" | "inactive" | "in_progress" | "queued" | "pending" | "success",
    targetUrl?: string,
    description?: string,
  ) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      await octokitResult.data.rest.repos.createDeploymentStatus({
        owner,
        repo,
        deployment_id: deploymentId,
        state,
        target_url: targetUrl,
        description,
      });

      return { data: undefined, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Get repository content (files/directories)
  async function getContent(
    installationId: number,
    owner: string,
    repo: string,
    path: string,
    ref?: string,
  ) {
    const octokitResult = await getInstallationOctokit(installationId);
    if (octokitResult.error) {
      return { data: null, error: octokitResult.error };
    }

    try {
      const { data } = await octokitResult.data.rest.repos.getContent({
        owner,
        repo,
        path,
        ref,
      });

      return { data, error: null };
    } catch (error) {
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  // Helper to iterate over all installations (useful for batch operations)
  async function* iterateInstallations() {
    for await (const { octokit, installation } of app.eachInstallation.iterator()) {
      yield { octokit, installation };
    }
  }

  // Helper to iterate over all repositories in an installation
  async function* iterateRepositories(installationId?: number) {
    const query = installationId ? { installationId } : undefined;
    for await (const { octokit, repository } of app.eachRepository.iterator(query)) {
      yield { octokit, repository };
    }
  }

  // OAuth utilities for user authentication
  async function exchangeCodeForToken(code: string) {
    try {
      // Use the built-in OAuth functionality from the GitHub App
      const result = await app.oauth.createToken({
        code,
      });

      return { data: result.authentication, error: null };
    } catch (error) {
      if (error instanceof RequestError) {
        return {
          data: null,
          error: new Error(`OAuth token exchange failed (${error.status}): ${error.message}`),
        };
      }
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  async function getUserWithToken(accessToken: string) {
    try {
      // Create a new Octokit instance with the user's access token
      const userOctokit = new Octokit({ auth: accessToken });

      const { data: githubUser } = await userOctokit.rest.users.getAuthenticated();

      // Get user email if not public
      if (!githubUser.email) {
        const { data: emails } = await userOctokit.rest.users.listEmailsForAuthenticatedUser();
        const primaryEmail = emails.find((e) => e.primary && e.verified);
        if (primaryEmail) {
          githubUser.email = primaryEmail.email;
        }
      }

      return { data: githubUser, error: null };
    } catch (error) {
      if (error instanceof RequestError) {
        return {
          data: null,
          error: new Error(`Failed to get user data (${error.status}): ${error.message}`),
        };
      }
      return {
        data: null,
        error: error instanceof Error ? error : new Error(`Unknown error: ${error}`),
      };
    }
  }

  async function listInstallations(userId: string) {
    const { data } = await app.octokit.rest.apps.listInstallationsForAuthenticatedUser({});
    return { data, error: null };
  }

  return {
    // Installation management
    installation: {
      getToken: getInstallationToken,
      getOctokit: getInstallationOctokit,
      iterate: iterateInstallations,
      list: listInstallations,
    },

    // Repository operations
    repository: {
      get: getRepository,
      listBranches,
      listCollaborators,
      dispatchEvent: dispatchRepositoryEvent,
      getContent,
      iterate: iterateRepositories,
    },

    // Branch operations
    branch: {
      getLatestCommitSHA,
    },

    // Commit operations
    commit: {
      get: getCommit,
    },

    // Deployment operations
    deployment: {
      create: createDeployment,
      updateStatus: createDeploymentStatus,
    },

    // OAuth operations
    oauth: {
      exchangeCodeForToken,
      getUserWithToken,
    },

    // Direct access to the App instance (if needed for advanced use cases)
    app,
  };
}
