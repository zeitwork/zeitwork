import {
  organisations,
  users,
  organisationMembers,
  githubInstallations,
} from "@zeitwork/database/schema";
import { eq } from "~~/server/utils/drizzle";

// Helper function to log structured errors
function logAuthError(context: string, error: any, additionalData?: Record<string, any>) {
  console.error(`[GitHub Auth Error - ${context}]`, {
    error:
      error instanceof Error
        ? {
            message: error.message,
            name: error.name,
            stack: error.stack,
            cause: error.cause,
          }
        : error,
    ...additionalData,
    timestamp: new Date().toISOString(),
  });
}

export default defineOAuthGitHubEventHandler({
  config: {
    emailRequired: true,
  },
  async onSuccess(event, { user, tokens }) {
    const startTime = Date.now();
    let dbUser: any = null;
    let organisationId: string | null = null;

    try {
      // Validate required user data from GitHub
      if (!user.id) {
        logAuthError("Validation", new Error("Missing GitHub user ID"), { user });
        throw createError({
          statusCode: 400,
          statusMessage: "Invalid GitHub user data: missing user ID",
        });
      }

      if (!user.login) {
        logAuthError("Validation", new Error("Missing GitHub username"), { user });
        throw createError({
          statusCode: 400,
          statusMessage: "Invalid GitHub user data: missing username",
        });
      }

      console.log(
        `[GitHub Auth] Starting authentication for GitHub user: ${user.login} (ID: ${user.id})`,
      );

      // Check if user exists
      try {
        dbUser = await useDrizzle()
          .select()
          .from(users)
          .where(eq(users.githubAccountId, user.id))
          .limit(1)
          .then((rows) => rows[0]);

        console.log(`[GitHub Auth] User lookup result: ${dbUser ? "existing user" : "new user"}`);
      } catch (error) {
        logAuthError("User Lookup", error, { githubUserId: user.id });
        throw createError({
          statusCode: 500,
          statusMessage: "Failed to query user database",
        });
      }

      // Only find default organisation if user exists
      if (dbUser) {
        try {
          const [defaultOrganisation] = await useDrizzle()
            .select()
            .from(organisations)
            .innerJoin(
              organisationMembers,
              eq(organisations.id, organisationMembers.organisationId),
            )
            .where(eq(organisationMembers.userId, dbUser.id))
            .limit(1);

          if (defaultOrganisation) {
            organisationId = defaultOrganisation.organisations.id;
            console.log(`[GitHub Auth] Found default organisation: ${organisationId}`);
          }
        } catch (error) {
          logAuthError("Organisation Lookup", error, { userId: dbUser.id });
          // Don't throw here, we can continue without the organisation
          console.warn(
            `[GitHub Auth] Failed to fetch organisation for user ${dbUser.id}, continuing...`,
          );
        }
      }

      if (!dbUser) {
        console.log(`[GitHub Auth] Creating new user for GitHub user: ${user.login}`);

        // Create new user
        try {
          const [newUser] = await useDrizzle()
            .insert(users)
            .values({
              name: user.name || user.login,
              email: user.email || `${user.login}@users.noreply.github.com`,
              username: user.login,
              githubAccountId: user.id,
            })
            .returning();

          dbUser = newUser;

          if (!dbUser) {
            throw new Error("User insert returned no data");
          }

          console.log(`[GitHub Auth] Created new user with ID: ${dbUser.id}`);
        } catch (error) {
          logAuthError("User Creation", error, {
            githubUser: {
              id: user.id,
              login: user.login,
              email: user.email,
            },
          });
          throw createError({
            statusCode: 500,
            statusMessage: "Failed to create user account",
          });
        }

        // Create default organisation for the user
        try {
          const [organisation] = await useDrizzle()
            .insert(organisations)
            .values({
              name: user.login,
              slug: user.login.toLowerCase(),
            })
            .returning();

          if (!organisation) {
            throw new Error("Organisation insert returned no data");
          }

          organisationId = organisation.id;
          console.log(`[GitHub Auth] Created organisation: ${organisationId}`);

          // Add user to their organisation
          await useDrizzle().insert(organisationMembers).values({
            userId: dbUser.id,
            organisationId: organisation.id,
          });

          console.log(`[GitHub Auth] Added user to organisation`);
        } catch (error) {
          logAuthError("Organisation Creation", error, { userId: dbUser.id });
          // Don't throw here - user was created, we can continue
          console.warn(
            `[GitHub Auth] Failed to create organisation for user ${dbUser.id}, continuing...`,
          );
        }
      }

      if (!dbUser) {
        logAuthError("User Retrieval", new Error("User object is null after creation/retrieval"));
        throw createError({
          statusCode: 500,
          statusMessage: "Failed to create or retrieve user",
        });
      }

      // Check subscription status (non-blocking)
      let hasSubscription = false;
      if (organisationId) {
        try {
          hasSubscription = await hasValidSubscription(organisationId);
          console.log(`[GitHub Auth] Subscription check: ${hasSubscription}`);
        } catch (error) {
          logAuthError("Subscription Check", error, { organisationId });
          console.warn(`[GitHub Auth] Failed to check subscription status, continuing...`);
        }
      }

      // Set user session
      try {
        await setUserSession(event, {
          user: {
            id: dbUser.id,
            name: dbUser.name,
            email: dbUser.email,
            username: dbUser.username,
            githubId: dbUser.githubAccountId,
            avatarUrl: user.avatar_url,
            verifiedAt: dbUser.verifiedAt,
          },
          secure: {
            userId: dbUser.id,
            organisationId: organisationId,
            tokens: tokens,
          },
          hasSubscription,
          subscriptionCheckedAt: Date.now(),
        });
        console.log(`[GitHub Auth] Session created for user: ${dbUser.username}`);
      } catch (error) {
        logAuthError("Session Creation", error, { userId: dbUser.id });
        throw createError({
          statusCode: 500,
          statusMessage: "Failed to create user session",
        });
      }

      // Check for pending GitHub App installation
      const pendingInstallation = getCookie(event, "pending_installation");
      if (pendingInstallation) {
        console.log(`[GitHub Auth] Processing pending installation: ${pendingInstallation}`);

        // Clear the cookie
        deleteCookie(event, "pending_installation");

        try {
          // Find the default organisation for the user
          const [organisation] = await useDrizzle()
            .select()
            .from(organisations)
            .innerJoin(
              organisationMembers,
              eq(organisations.id, organisationMembers.organisationId),
            )
            .where(eq(organisationMembers.userId, dbUser.id))
            .limit(1);

          if (!organisation) {
            logAuthError("Installation Processing", new Error("Organisation not found"), {
              userId: dbUser.id,
              installationId: pendingInstallation,
            });
            throw createError({
              statusCode: 500,
              statusMessage: "Failed to find user's organisation for GitHub installation",
            });
          }

          // Validate installation ID
          const installationId = parseInt(pendingInstallation);
          if (isNaN(installationId) || installationId <= 0) {
            logAuthError("Installation Processing", new Error("Invalid installation ID"), {
              pendingInstallation,
              parsed: installationId,
            });
            throw createError({
              statusCode: 400,
              statusMessage: "Invalid GitHub installation ID",
            });
          }

          // Upsert the GitHub installation record
          await useDrizzle()
            .insert(githubInstallations)
            .values({
              githubInstallationId: installationId,
              organisationId: organisation.organisations.id,
              githubAccountId: dbUser.githubAccountId!,
              userId: dbUser.id,
            })
            .onConflictDoUpdate({
              target: [githubInstallations.githubInstallationId],
              set: {
                organisationId: organisation.organisations.id,
                userId: dbUser.id,
              },
            });

          console.log(
            `[GitHub Auth] GitHub installation ${installationId} linked to organisation ${organisation.organisations.id}`,
          );

          const duration = Date.now() - startTime;
          console.log(
            `[GitHub Auth] Authentication completed successfully in ${duration}ms (with installation)`,
          );

          return sendRedirect(event, `/${dbUser.username}?installed=true`);
        } catch (error) {
          logAuthError("Installation Processing", error, {
            userId: dbUser.id,
            installationId: pendingInstallation,
          });
          // If installation processing fails, still redirect to user page
          console.warn(
            `[GitHub Auth] Installation processing failed, redirecting to user page anyway`,
          );
          return sendRedirect(event, `/${dbUser.username}?installation_error=true`);
        }
      }

      const duration = Date.now() - startTime;
      console.log(`[GitHub Auth] Authentication completed successfully in ${duration}ms`);

      return sendRedirect(event, `/${dbUser.username}`);
    } catch (error) {
      const duration = Date.now() - startTime;
      logAuthError("General", error, {
        duration,
        userId: dbUser?.id,
        organisationId,
        hasUser: !!dbUser,
      });

      // If we have a H3Error, rethrow it
      if (error && typeof error === "object" && "statusCode" in error) {
        throw error;
      }

      // Otherwise throw a generic error
      throw createError({
        statusCode: 500,
        statusMessage: "Authentication failed due to an unexpected error",
      });
    }
  },
  // Enhanced error handler for OAuth flow errors
  onError(event, error) {
    // Determine error type and provide appropriate feedback
    let errorMessage = "Authentication failed";
    let errorContext = "Unknown";

    if (error && typeof error === "object") {
      const err = error as any;

      // Network/fetch errors (like in the logs)
      if (err.name === "FetchError" || err.cause?.code === "UND_ERR_CONNECT_TIMEOUT") {
        errorContext = "Network Error";
        errorMessage = "Unable to connect to GitHub";
        logAuthError("OAuth Token Exchange", error, {
          type: "network_timeout",
          url: err.request?.url,
        });
      }
      // GitHub API errors
      else if (err.statusCode === 401 || err.statusCode === 403) {
        errorContext = "Authentication Error";
        errorMessage = "GitHub authentication failed";
        logAuthError("OAuth Token Exchange", error, {
          type: "auth_failed",
          statusCode: err.statusCode,
        });
      }
      // Rate limiting
      else if (err.statusCode === 429) {
        errorContext = "Rate Limit";
        errorMessage = "Too many authentication attempts";
        logAuthError("OAuth Token Exchange", error, {
          type: "rate_limit",
        });
      }
      // Invalid code or state
      else if (err.statusCode === 400) {
        errorContext = "Invalid Request";
        errorMessage = "Invalid authentication code";
        logAuthError("OAuth Token Exchange", error, {
          type: "invalid_code",
        });
      }
      // Generic error
      else {
        logAuthError("OAuth Flow", error, {
          type: "unknown",
          statusCode: err.statusCode,
          statusMessage: err.statusMessage,
        });
      }
    } else {
      logAuthError("OAuth Flow", error, { type: "unknown" });
    }

    console.error(`[GitHub OAuth Error - ${errorContext}]:`, errorMessage, error);

    // Redirect to login with error message
    return sendRedirect(event, `/login?error=${encodeURIComponent(errorMessage)}`);
  },
});
