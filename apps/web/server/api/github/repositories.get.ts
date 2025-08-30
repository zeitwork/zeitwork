import { githubInstallations, users } from "@zeitwork/database/schema"
import { z } from "zod"

const querySchema = z.object({
  account: z.string().min(1).max(255).optional(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const query = await getValidatedQuery(event, querySchema.parse)

  const [user] = await useDrizzle().select().from(users).where(eq(users.id, secure.userId)).limit(1)

  // List all github organisations for the user
  const installations = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.organisationId, secure.organisationId))

  // const octokit = new Octokit({ auth: secure.tokens.access_token })

  const github = useGitHub()

  let results: { id: number; account: string; avatarUrl: string }[] = []

  // Process all installations concurrently using Promise.all
  const repositoryPromises = installations.map(async (installation) => {
    const { data: octokit, error: octokitError } = await github.installation.getOctokit(
      installation.githubInstallationId,
    )

    if (octokitError) {
      console.error(octokitError)
      return []
    }

    const { data: listResult } = await octokit.rest.apps.listReposAccessibleToInstallation({
      installation_id: installation.githubInstallationId,
      per_page: 100,
    })

    return listResult.repositories.map((el: any) => ({
      id: el.id,
      name: el.name,
      fullName: el.full_name,
      ownerName: el.owner.login,
      account: el.owner.login,
    }))
  })

  const repositoryResults = await Promise.all(repositoryPromises)
  let repositories = repositoryResults.flat()

  //   filter by account
  if (query.account) {
    repositories = repositories.filter((repository) => repository.account === query.account)
  }

  return repositories
})
