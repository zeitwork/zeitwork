import { useDrizzle, eq } from "../../utils/drizzle"
import * as schema from "~~/packages/database/schema"

export default defineOAuthGitHubEventHandler({
  config: {
    emailRequired: true,
  },
  async onSuccess(event, { user, tokens }) {
    // console.log("GitHub OAuth success:", user, tokens)

    const db = useDrizzle()

    try {
      // Check if user exists
      let dbUser = await db
        .select()
        .from(schema.users)
        .where(eq(schema.users.githubId, user.id))
        .limit(1)
        .then((rows) => rows[0])

      if (!dbUser) {
        // Create new user
        const [newUser] = await db
          .insert(schema.users)
          .values({
            name: user.name || user.login,
            email: user.email || `${user.login}@users.noreply.github.com`,
            username: user.login,
            githubId: user.id,
          })
          .returning()

        dbUser = newUser

        if (dbUser) {
          // Create default organisation for the user
          const [organisation] = await db
            .insert(schema.organisations)
            .values({
              name: user.login,
              slug: user.login.toLowerCase(),
            })
            .returning()

          if (organisation) {
            // Add user to their organisation
            await db.insert(schema.organisationMembers).values({
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
          githubId: dbUser.githubId,
          avatarUrl: user.avatar_url,
        },
        secure: {
          userId: dbUser.id,
        },
      })

      // Check for pending GitHub App installation
      const pendingInstallation = getCookie(event, "pending_installation")
      if (pendingInstallation) {
        // Clear the cookie
        deleteCookie(event, "pending_installation")

        // Update the user's default organisation with the installation ID
        await db
          .update(schema.organisations)
          .set({ installationId: parseInt(pendingInstallation) })
          .where(eq(schema.organisations.slug, dbUser.username.toLowerCase()))

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
