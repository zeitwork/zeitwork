import { deployments, domains, projects } from "@zeitwork/database/schema"
import { eq, SQL } from "drizzle-orm"
import { z } from "zod"

const querySchema = z.object({
  projectSlug: z.string().optional(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { projectSlug } = await getValidatedQuery(event, querySchema.parse)

  // Get all deployments for the organisation
  let wheres: SQL[] = [eq(deployments.organisationId, secure.organisationId)]

  if (projectSlug) {
    const [project] = await useDrizzle()
      .select()
      .from(projects)
      .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
      .limit(1)

    if (project) {
      wheres.push(eq(deployments.projectId, project.id))
    }
  }

  const query = await useDrizzle()
    .select()
    .from(deployments)
    .leftJoin(domains, eq(deployments.id, domains.deploymentId))
    .where(and(...wheres))
    .orderBy(desc(deployments.id))

  const result = query.map((row) => ({
    ...row.deployments,
    domains: [row.domains],
  }))

  return result
})
