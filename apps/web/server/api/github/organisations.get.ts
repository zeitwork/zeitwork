import { githubInstallations, users } from "@zeitwork/database/schema"
import { Octokit } from "octokit"

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const [user] = await useDrizzle().select().from(users).where(eq(users.id, secure.userId)).limit(1)

  // List all github organisations for the user
  const installations = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.organisationId, secure.organisationId))

  // const octokit = new Octokit({ auth: secure.tokens.access_token })

  const github = useGitHub()

  let results: { id: number; account: string; avatarUrl: string }[] = []

  for (const installation of installations) {
    const { data: octokit, error: octokitError } = await github.installation.getOctokit(
      installation.githubInstallationId,
    )
    if (octokitError) {
      console.error(octokitError)
      continue
    }

    try {
      const { data: result } = await octokit.rest.apps.getInstallation({
        installation_id: installation.githubInstallationId,
      })
      if (!result) {
        continue
      }
      results.push({
        id: result.id,
        account: result?.account?.login,
        avatarUrl: result?.account?.avatar_url,
        // _: result,
      })
    } catch (error) {
      console.error(error)
    }
  }

  // let results = []

  // for (const installation of installations) {
  //   const octokit = await github.app.getInstallationOctokit(installation.githubInstallationId)
  //   const installations = await octokit.rest.apps.listInstallationReposForAuthenticatedUser()
  //   results.push(installations.data)
  // }

  return results
})
