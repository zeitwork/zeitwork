import { $fetch } from "ofetch"
import { useDrizzle, eq } from "../../utils/drizzle"
import * as schema from "~~/packages/database/schema"

interface GitHubAccessTokenResponse {
  access_token: string
  token_type: string
  scope: string
}

interface GitHubUser {
  id: number
  login: string
  name: string | null
  email: string | null
  avatar_url: string
}

export default defineEventHandler(async (event) => {
  const { code, installation_id, setup_action, state } = getQuery(event)

  if (!code) {
    throw createError({
      statusCode: 400,
      statusMessage: "Missing authorization code",
    })
  }

  const config = useRuntimeConfig()

  try {
    // Exchange code for access token
    const tokenResponse = await $fetch<GitHubAccessTokenResponse>("https://github.com/login/oauth/access_token", {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: {
        client_id: config.oauth.github.clientId,
        client_secret: config.oauth.github.clientSecret,
        code: code as string,
      },
    })

    if (!tokenResponse.access_token) {
      throw new Error("Failed to get access token")
    }

    // Get user information
    const githubUser = await $fetch<GitHubUser>("https://api.github.com/user", {
      headers: {
        Authorization: `Bearer ${tokenResponse.access_token}`,
        Accept: "application/vnd.github.v3+json",
      },
    })

    // Get user email if not public
    if (!githubUser.email) {
      const emails = await $fetch<Array<{ email: string; primary: boolean; verified: boolean }>>(
        "https://api.github.com/user/emails",
        {
          headers: {
            Authorization: `Bearer ${tokenResponse.access_token}`,
            Accept: "application/vnd.github.v3+json",
          },
        },
      )

      const primaryEmail = emails.find((e) => e.primary && e.verified)
      if (primaryEmail) {
        githubUser.email = primaryEmail.email
      }
    }

    const db = useDrizzle()

    // Check if user exists
    let dbUser = await db
      .select()
      .from(schema.users)
      .where(eq(schema.users.githubId, githubUser.id))
      .limit(1)
      .then((rows) => rows[0])

    if (!dbUser) {
      // Create new user
      const [newUser] = await db
        .insert(schema.users)
        .values({
          name: githubUser.name || githubUser.login,
          email: githubUser.email || `${githubUser.login}@users.noreply.github.com`,
          username: githubUser.login,
          githubId: githubUser.id,
        })
        .returning()

      dbUser = newUser

      if (dbUser) {
        // Create default organisation for the user
        const [organisation] = await db
          .insert(schema.organisations)
          .values({
            name: githubUser.login,
            slug: githubUser.login.toLowerCase(),
            // If installation_id is present, set it
            installationId: installation_id ? parseInt(installation_id as string) : undefined,
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
    } else if (installation_id) {
      // User exists but we have a new installation
      // Update the organisation specified in state, or default org
      const orgSlug = (state as string) || dbUser.username.toLowerCase()

      await db
        .update(schema.organisations)
        .set({ installationId: parseInt(installation_id as string) })
        .where(eq(schema.organisations.slug, orgSlug))
    }

    if (!dbUser) {
      throw createError({
        statusCode: 500,
        statusMessage: "Failed to create or retrieve user",
      })
    }

    // Set user session
    await setUserSession(event, {
      user: {
        id: dbUser.id,
        name: dbUser.name,
        email: dbUser.email,
        username: dbUser.username,
        githubId: dbUser.githubId,
        avatarUrl: githubUser.avatar_url,
      },
      secure: {
        userId: dbUser.id,
      },
    })

    // Redirect based on context
    if (installation_id && setup_action === "install") {
      // New installation, redirect with success message
      const orgSlug = (state as string) || dbUser.username
      return sendRedirect(event, `/${orgSlug}?installed=true`)
    } else {
      // Regular login
      return sendRedirect(event, `/${dbUser.username}`)
    }
  } catch (error) {
    console.error("GitHub callback error:", error)
    throw createError({
      statusCode: 500,
      statusMessage: "Authentication failed",
    })
  }
})
