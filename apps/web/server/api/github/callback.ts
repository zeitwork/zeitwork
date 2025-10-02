import { users, organisations, organisationMembers, githubInstallations } from "@zeitwork/database/schema"
import type { H3Event } from "h3"
import { useDrizzle, eq } from "../../utils/drizzle"
import { useGitHub } from "../../utils/github"
import { Octokit } from "octokit"
import { z } from "zod"

const querySchema = z.object({
  installation_id: z.coerce.number(),
  setup_action: z.enum(["install", "update"]),
})

export default defineEventHandler(async (event) => {
  try {
    const session = await requireUserSession(event)
    const { secure } = session

    if (!secure) {
      throw createError({ statusCode: 401, message: "Unauthorized" })
    }

    // Validate query parameters
    let query
    try {
      query = await getValidatedQuery(event, querySchema.parse)
    } catch (error: any) {
      throw createError({
        statusCode: 400,
        message: "Invalid GitHub callback parameters",
      })
    }

    const { installation_id: installationId, setup_action: setupAction } = query

    // Get user
    let user
    try {
      const [foundUser] = await useDrizzle().select().from(users).where(eq(users.id, secure.userId)).limit(1)
      user = foundUser
    } catch (error) {
      console.error("[GitHub API - callback] Database error fetching user:", error)
      throw createError({
        statusCode: 500,
        message: "Failed to fetch user data",
      })
    }

    if (!user) {
      throw createError({
        statusCode: 404,
        message: "User not found",
      })
    }

    if (!user.githubAccountId) {
      throw createError({
        statusCode: 400,
        message: "User is not linked to a GitHub account",
      })
    }

    // Get organisation
    let organisation
    try {
      const [foundOrg] = await useDrizzle()
        .select()
        .from(organisations)
        .where(eq(organisations.id, secure.organisationId))
        .limit(1)
      organisation = foundOrg
    } catch (error) {
      console.error("[GitHub API - callback] Database error fetching organisation:", error)
      throw createError({
        statusCode: 500,
        message: "Failed to fetch organisation data",
      })
    }

    if (!organisation) {
      throw createError({
        statusCode: 404,
        message: "Organisation not found",
      })
    }

    // Verify installation
    const { data: verifiedInstallation, error: verifiedInstallationError } = await verifyInstallationForUser({
      event,
      installationId,
    })

    if (verifiedInstallationError) {
      console.error(
        `[GitHub API - callback] Installation verification failed for installation ${installationId}:`,
        verifiedInstallationError,
      )
      throw createError({
        statusCode: 403,
        message: "Unable to verify GitHub installation. Please ensure the app is properly installed.",
      })
    }

    if (!verifiedInstallation) {
      throw createError({
        statusCode: 404,
        message: "GitHub installation not found",
      })
    }

    // Process setup action
    try {
      switch (setupAction) {
        case "install":
          await useDrizzle()
            .insert(githubInstallations)
            .values({
              userId: secure.userId,
              githubAccountId: user.githubAccountId,
              githubInstallationId: installationId,
              organisationId: secure.organisationId,
            })
            .onConflictDoUpdate({
              target: [githubInstallations.githubInstallationId],
              set: {
                organisationId: secure.organisationId,
              },
            })

          return sendRedirect(event, `/${organisation.slug}?installed=true`)

        case "update":
          await useDrizzle()
            .insert(githubInstallations)
            .values({
              organisationId: secure.organisationId,
              githubInstallationId: installationId,
              githubAccountId: user.githubAccountId,
              userId: secure.userId,
            })
            .onConflictDoUpdate({
              target: [githubInstallations.githubInstallationId],
              set: {
                organisationId: secure.organisationId,
              },
            })

          return sendRedirect(event, `/${organisation.slug}?installed=true`)
      }
    } catch (error) {
      console.error(`[GitHub API - callback] Database error during ${setupAction}:`, error)
      throw createError({
        statusCode: 500,
        message: "Failed to save GitHub installation",
      })
    }
  } catch (error: any) {
    // If it's already an H3Error, rethrow it
    if (error.statusCode) {
      throw error
    }

    throw createError({
      statusCode: 500,
      message: "An unexpected error occurred during GitHub installation",
    })
  }
})

async function verifyInstallationForUser({
  event,
  installationId,
}: {
  event: H3Event
  installationId: number
}): Promise<{ data: any; error: null } | { data: null; error: Error }> {
  try {
    const userSession = await getUserSession(event)

    if (!userSession?.secure?.tokens?.access_token) {
      return { data: null, error: new Error("Missing authentication token") }
    }

    const octokit = new Octokit({ auth: userSession.secure.tokens.access_token })

    let userInstallationResult
    try {
      const response = await octokit.rest.apps.listInstallationsForAuthenticatedUser()
      userInstallationResult = response.data
    } catch (error: any) {
      console.error("[GitHub API - verifyInstallation] Failed to list installations:", error)
      return {
        data: null,
        error: new Error(`Failed to fetch installations: ${error.message || "Unknown error"}`),
      }
    }

    const installations = userInstallationResult.installations

    if (!installations || installations.length === 0) {
      return { data: null, error: new Error("No GitHub installations found") }
    }

    const installation = installations.find((installation) => installation.id === installationId)

    if (!installation) {
      return { data: null, error: new Error("Installation not found for this user") }
    }

    return { data: installation, error: null }
  } catch (error: any) {
    console.error("[GitHub API - verifyInstallation] Unexpected error:", error)
    return {
      data: null,
      error: new Error(`Verification failed: ${error.message || "Unknown error"}`),
    }
  }
}
