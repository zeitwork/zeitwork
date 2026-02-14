import {
  organisations,
  users,
  organisationMembers,
  githubInstallations,
} from "@zeitwork/database/schema";
import { eq } from "~~/server/utils/drizzle";
import { addSeconds } from "date-fns";

export default defineOAuthGitHubEventHandler({
  config: {
    emailRequired: true,
  },
  async onSuccess(event, { user, tokens }) {
    let dbUser: any = null;
    let organisationId: string | null = null;

    try {
      // Validate required user data from GitHub
      if (!user.id) {
        throw createError({
          statusCode: 400,
          statusMessage: "Invalid GitHub user data: missing user ID",
        });
      }

      if (!user.login) {
        throw createError({
          statusCode: 400,
          statusMessage: "Invalid GitHub user data: missing username",
        });
      }

      // Check if user exists
      try {
        dbUser = await useDrizzle()
          .select()
          .from(users)
          .where(eq(users.githubAccountId, user.id))
          .limit(1)
          .then((rows) => rows[0]);
      } catch {
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
          }
        } catch {
          // Don't throw here, we can continue without the organisation
        }
      }

      if (!dbUser) {
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
        } catch {
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

          // Add user to their organisation
          await useDrizzle().insert(organisationMembers).values({
            userId: dbUser.id,
            organisationId: organisation.id,
          });
        } catch {
          // Don't throw here - user was created, we can continue
        }
      }

      if (!dbUser) {
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
        } catch {
          // Non-blocking, continue without subscription info
        }
      }

      // Calculate token expiry (GitHub App tokens include expires_in but the library type omits it)
      const expiresInSeconds = "expires_in" in tokens ? (tokens.expires_in as number) : 28800;
      const tokenExpiresAt = addSeconds(new Date(), expiresInSeconds).getTime();

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
            tokenExpiresAt,
          },
          hasSubscription,
          subscriptionCheckedAt: Date.now(),
        });
      } catch {
        throw createError({
          statusCode: 500,
          statusMessage: "Failed to create user session",
        });
      }

      // Check for pending GitHub App installation
      const pendingInstallation = getCookie(event, "pending_installation");
      if (pendingInstallation) {
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
            throw createError({
              statusCode: 500,
              statusMessage: "Failed to find user's organisation for GitHub installation",
            });
          }

          // Validate installation ID
          const installationId = parseInt(pendingInstallation);
          if (isNaN(installationId) || installationId <= 0) {
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

          return sendRedirect(event, `/${dbUser.username}?installed=true`);
        } catch {
          // If installation processing fails, still redirect to user page
          return sendRedirect(event, `/${dbUser.username}?installation_error=true`);
        }
      }

      return sendRedirect(event, `/${dbUser.username}`);
    } catch (error) {
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
  onError(event, _error) {
    return sendRedirect(event, `/login?error=${encodeURIComponent("Authentication failed")}`);
  },
});
