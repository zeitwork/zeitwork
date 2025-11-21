import { deployments, domains, projects } from "@zeitwork/database/schema"
import { eq, SQL, inArray, desc, and } from "@zeitwork/database/utils/drizzle"
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

  // Query 1: Get deployments (no joins, no duplicates)
  const deploymentsResult = await useDrizzle()
    .select()
    .from(deployments)
    .where(and(...wheres))
    .orderBy(desc(deployments.id))

  // Query 2: Get all domains for these deployments in one query
  const deploymentIds = deploymentsResult.map((d) => d.id)

  let domainsResult: any[] = []
  if (deploymentIds.length > 0) {
    domainsResult = await useDrizzle().select().from(domains).where(inArray(domains.deploymentId, deploymentIds))
  }

  // Group domains by deploymentId in memory (fast operation)
  const domainsByDeployment = new Map()
  for (const domain of domainsResult) {
    if (!domainsByDeployment.has(domain.deploymentId)) {
      domainsByDeployment.set(domain.deploymentId, [])
    }
    domainsByDeployment.get(domain.deploymentId).push(domain)
  }

  // Attach domains to deployments
  const result = deploymentsResult.map((deployment) => ({
    ...deployment,
    domains: domainsByDeployment.get(deployment.id) || [],
  }))

  return result
})
