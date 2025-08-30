import { organisations, users, organisationMembers, githubInstallations } from "@zeitwork/database/schema"
import { eq } from "~~/server/utils/drizzle"

export default defineOAuthGitHubEventHandler({
  config: {
    emailRequired: true,
  },
  async onSuccess(event, { user, tokens }) {
    try {
      let organisationId = null

      // Check if user exists
      let dbUser = await useDrizzle()
        .select()
        .from(users)
        .where(eq(users.githubAccountId, user.id))
        .limit(1)
        .then((rows) => rows[0])

      // Only find default organisation if user exists
      if (dbUser) {
        const [defaultOrganisation] = await useDrizzle()
          .select()
          .from(organisations)
          .innerJoin(organisationMembers, eq(organisations.id, organisationMembers.organisationId))
          .where(eq(organisationMembers.userId, dbUser.id))
          .limit(1)

        if (defaultOrganisation) {
          organisationId = defaultOrganisation.organisations.id
        }
      }

      if (!dbUser) {
        // Create new user
        const [newUser] = await useDrizzle()
          .insert(users)
          .values({
            name: user.name || user.login,
            email: user.email || `${user.login}@users.noreply.github.com`,
            username: user.login,
            githubAccountId: user.id,
          })
          .returning()

        dbUser = newUser

        if (dbUser) {
          // Create default organisation for the user
          const [organisation] = await useDrizzle()
            .insert(organisations)
            .values({
              name: user.login,
              slug: user.login.toLowerCase(),
            })
            .returning()

          organisationId = organisation.id

          if (organisation) {
            // Add user to their organisation
            await useDrizzle().insert(organisationMembers).values({
              userId: dbUser.id,
              organisationId: organisation.id,
            })
          }
        }
      }

      if (!dbUser) {
        throw createError({
          statusCode: 500,
          statusMessage: "Failed to create or retrieve user",
        })
      }

      await setUserSession(event, {
        user: {
          id: dbUser.id,
          name: dbUser.name,
          email: dbUser.email,
          username: dbUser.username,
          githubId: dbUser.githubAccountId,
          avatarUrl: user.avatar_url,
        },
        secure: {
          userId: dbUser.id,
          organisationId: organisationId,
          tokens: tokens,
        },
      })

      // Check for pending GitHub App installation
      const pendingInstallation = getCookie(event, "pending_installation")
      if (pendingInstallation) {
        // Clear the cookie
        deleteCookie(event, "pending_installation")

        // Find the default organisation for the user
        const [organisation] = await useDrizzle()
          .select()
          .from(organisations)
          .innerJoin(organisationMembers, eq(organisations.id, organisationMembers.organisationId))
          .where(eq(organisationMembers.userId, dbUser.id))
          .limit(1)
        if (!organisation) {
          throw createError({
            statusCode: 500,
            statusMessage: "Failed to find user's default organisation",
          })
        }

        // Upsert the GitHub installation record
        await useDrizzle()
          .insert(githubInstallations)
          .values({
            githubInstallationId: parseInt(pendingInstallation),
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
          })

        return sendRedirect(event, `/${dbUser.username}?installed=true`)
      }

      return sendRedirect(event, `/${dbUser.username}`)
    } catch (error) {
      console.error("Database error during authentication:", error)
      throw createError({
        statusCode: 500,
        statusMessage: "Authentication failed",
      })
    }
  },
  // Optional, will return a json error and 401 status code by default
  onError(event, error) {
    console.error("GitHub OAuth error:", error)
    return sendRedirect(event, "/login")
  },
})
