import { githubInstallations, users } from "@zeitwork/database/schema";
import { z } from "zod";

const querySchema = z.object({
  account: z.string().min(1).max(255).optional(),
});

export default defineEventHandler(async (event) => {
  try {
    const { secure } = await requireUserSession(event);
    if (!secure) {
      console.error("[GitHub API - repositories.get] Missing session data");
      throw createError({ statusCode: 401, message: "Unauthorized" });
    }

    // Validate query parameters
    let query;
    try {
      query = await getValidatedQuery(event, querySchema.parse);
    } catch (error: any) {
      console.error("[GitHub API - repositories.get] Invalid query parameters:", error);
      throw createError({
        statusCode: 400,
        message: "Invalid query parameters",
      });
    }

    // Get user
    let user;
    try {
      const [foundUser] = await useDrizzle()
        .select()
        .from(users)
        .where(eq(users.id, secure.userId))
        .limit(1);
      user = foundUser;
    } catch (error) {
      console.error("[GitHub API - repositories.get] Database error fetching user:", error);
      throw createError({
        statusCode: 500,
        message: "Failed to fetch user data",
      });
    }

    if (!user) {
      console.error("[GitHub API - repositories.get] User not found:", secure.userId);
      throw createError({
        statusCode: 404,
        message: "User not found",
      });
    }

    // List all github installations for the user's organisation
    let installations;
    try {
      installations = await useDrizzle()
        .select()
        .from(githubInstallations)
        .where(eq(githubInstallations.organisationId, secure.organisationId));
    } catch (error) {
      console.error(
        "[GitHub API - repositories.get] Database error fetching installations:",
        error,
      );
      throw createError({
        statusCode: 500,
        message: "Failed to fetch GitHub installations",
      });
    }

    if (installations.length === 0) {
      console.log(
        "[GitHub API - repositories.get] No installations found for organisation:",
        secure.organisationId,
      );
      return [];
    }

    const github = useGitHub();
    const errors: Array<{ installationId: number; error: any }> = [];

    // Process all installations concurrently using Promise.all
    const repositoryPromises = installations.map(async (installation) => {
      try {
        const { data: octokit, error: octokitError } = await github.installation.getOctokit(
          installation.githubInstallationId,
        );

        if (octokitError) {
          console.error(
            `[GitHub API - repositories.get] Failed to get Octokit for installation ${installation.githubInstallationId}:`,
            octokitError,
          );
          errors.push({
            installationId: installation.githubInstallationId,
            error: octokitError.message,
          });
          return [];
        }

        const { data: listResult } = await octokit.rest.apps.listReposAccessibleToInstallation({
          installation_id: installation.githubInstallationId,
          per_page: 100,
        });

        if (!listResult || !listResult.repositories) {
          console.warn(
            `[GitHub API - repositories.get] No repositories found for installation ${installation.githubInstallationId}`,
          );
          return [];
        }

        return listResult.repositories.map((el: any) => ({
          id: el.id,
          name: el.name,
          fullName: el.full_name,
          ownerName: el.owner.login,
          account: el.owner.login,
        }));
      } catch (error: any) {
        console.error(
          `[GitHub API - repositories.get] Failed to fetch repositories for installation ${installation.githubInstallationId}:`,
          error,
        );
        errors.push({
          installationId: installation.githubInstallationId,
          error: error.message || "Unknown error",
        });
        return [];
      }
    });

    const repositoryResults = await Promise.all(repositoryPromises);
    let repositories = repositoryResults.flat();

    if (errors.length > 0 && repositories.length === 0) {
      console.error("[GitHub API - repositories.get] All installations failed:", errors);
      throw createError({
        statusCode: 502,
        message: "Failed to fetch GitHub repositories. Please check your GitHub App installation.",
      });
    }

    if (errors.length > 0) {
      console.warn(
        `[GitHub API - repositories.get] Partial success: ${repositories.length} repositories fetched, ${errors.length} installations failed`,
      );
    }

    // Filter by account
    if (query.account) {
      const beforeFilter = repositories.length;
      repositories = repositories.filter((repository) => repository.account === query.account);
      console.log(
        `[GitHub API - repositories.get] Filtered from ${beforeFilter} to ${repositories.length} repositories for account: ${query.account}`,
      );
    }

    return repositories;
  } catch (error: any) {
    // If it's already an H3Error, rethrow it
    if (error.statusCode) {
      throw error;
    }

    console.error("[GitHub API - repositories.get] Unexpected error:", error);
    throw createError({
      statusCode: 500,
      message: "An unexpected error occurred while fetching GitHub repositories",
    });
  }
});
