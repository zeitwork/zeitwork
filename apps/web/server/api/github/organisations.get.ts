import { githubInstallations, users } from "@zeitwork/database/schema"

export default defineEventHandler(async (event) => {
  try {
    const { secure } = await requireUserSession(event)
    if (!secure) {
      console.error("[GitHub API - organisations.get] Missing session data")
      throw createError({ statusCode: 401, message: "Unauthorized" })
    }

    // Get user
    let user
    try {
      const [foundUser] = await useDrizzle().select().from(users).where(eq(users.id, secure.userId)).limit(1)
      user = foundUser
    } catch (error) {
      console.error("[GitHub API - organisations.get] Database error fetching user:", error)
      throw createError({
        statusCode: 500,
        message: "Failed to fetch user data",
      })
    }

    if (!user) {
      console.error("[GitHub API - organisations.get] User not found:", secure.userId)
      throw createError({
        statusCode: 404,
        message: "User not found",
      })
    }

    // List all github installations for the user's organisation
    let installations
    try {
      installations = await useDrizzle()
        .select()
        .from(githubInstallations)
        .where(eq(githubInstallations.organisationId, secure.organisationId))
    } catch (error) {
      console.error("[GitHub API - organisations.get] Database error fetching installations:", error)
      throw createError({
        statusCode: 500,
        message: "Failed to fetch GitHub installations",
      })
    }

    if (installations.length === 0) {
      console.log("[GitHub API - organisations.get] No installations found for organisation:", secure.organisationId)
      return []
    }

    const github = useGitHub()
    let results: { id: number; account: string; avatarUrl: string }[] = []
    const errors: Array<{ installationId: number; error: any }> = []

    for (const installation of installations) {
      const { data: octokit, error: octokitError } = await github.installation.getOctokit(
        installation.githubInstallationId,
      )
      if (octokitError) {
        console.error(
          `[GitHub API - organisations.get] Failed to get Octokit for installation ${installation.githubInstallationId}:`,
          octokitError,
        )
        errors.push({ installationId: installation.githubInstallationId, error: octokitError.message })
        continue
      }

      try {
        const { data: result } = await octokit.rest.apps.getInstallation({
          installation_id: installation.githubInstallationId,
        })
        if (!result) {
          console.warn(
            `[GitHub API - organisations.get] No result for installation ${installation.githubInstallationId}`,
          )
          continue
        }
        results.push({
          id: result.id,
          account: result?.account?.login,
          avatarUrl: result?.account?.avatar_url,
        })
      } catch (error: any) {
        console.error(
          `[GitHub API - organisations.get] Failed to fetch installation ${installation.githubInstallationId} details:`,
          error,
        )
        errors.push({
          installationId: installation.githubInstallationId,
          error: error.message || "Unknown error",
        })
      }
    }

    if (errors.length > 0 && results.length === 0) {
      console.error("[GitHub API - organisations.get] All installations failed:", errors)
      throw createError({
        statusCode: 502,
        message: "Failed to fetch GitHub organisations. Please check your GitHub App installation.",
      })
    }

    if (errors.length > 0) {
      console.warn(
        `[GitHub API - organisations.get] Partial success: ${results.length} succeeded, ${errors.length} failed`,
      )
    }

    return results
  } catch (error: any) {
    // If it's already an H3Error, rethrow it
    if (error.statusCode) {
      throw error
    }

    console.error("[GitHub API - organisations.get] Unexpected error:", error)
    throw createError({
      statusCode: 500,
      message: "An unexpected error occurred while fetching GitHub organisations",
    })
  }
})
