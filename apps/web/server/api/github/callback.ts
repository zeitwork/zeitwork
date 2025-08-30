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
  const session = await requireUserSession(event)
  const { secure } = session

  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { installation_id: installationId, setup_action: setupAction } = await getValidatedQuery(
    event,
    querySchema.parse,
  )

  const [user] = await useDrizzle().select().from(users).where(eq(users.id, secure.userId)).limit(1)
  if (!user.githubAccountId) {
    throw createError({ statusCode: 401, message: "Unauthorized" })
  }

  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.id, secure.organisationId))
    .limit(1)
  if (!organisation) {
    throw createError({ statusCode: 401, message: "Unauthorized" })
  }

  const { data: verfiedInstallation, error: verfiedInstallationError } = await verifyInstallationForUser({
    event,
    installationId,
  })
  if (verfiedInstallationError) {
    throw createError({ statusCode: 401, message: "Unauthorized" })
  }

  switch (setupAction) {
    case "install":
      // Create the installation record
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
      // Upsert the installation record
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
})

async function useOctokit({ accessToken, installationId }: { accessToken: string; installationId: number }) {
  return new Octokit({ auth: accessToken })
}

async function verifyInstallationForUser({
  event,
  installationId,
}: {
  event: H3Event
  installationId: number
}): Promise<{ data: any; error: null } | { data: null; error: Error }> {
  try {
    const userSession = await getUserSession(event)

    const octokit = new Octokit({ auth: userSession!.secure!.tokens.access_token })

    const { data: userInstallationResult } = await octokit.rest.apps.listInstallationsForAuthenticatedUser()

    const installations = userInstallationResult.installations

    if (!installations) {
      return { data: null, error: new Error("Unauthorized") }
    }

    const installation = installations.find((installation) => installation.id === installationId)

    return { data: installation, error: null }
  } catch (error) {
    return { data: null, error: new Error("Unauthorized") }
  }
}
