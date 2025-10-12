import { deployments, domains } from "@zeitwork/database/schema"
import { eq, and } from "drizzle-orm"

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const deploymentId = getRouterParam(event, "id")
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" })
  }

  // Get the deployment with domains
  const query = await useDrizzle()
    .select()
    .from(deployments)
    .leftJoin(domains, eq(deployments.id, domains.deploymentId))
    .where(
      and(
        eq(deployments.id, deploymentId),
        eq(deployments.organisationId, secure.organisationId)
      )
    )

  if (query.length === 0) {
    throw createError({ statusCode: 404, message: "Deployment not found" })
  }

  // Group domains with deployment
  const deployment = query[0].deployments
  const deploymentDomains = query.map(row => row.domains).filter(Boolean)

  return {
    ...deployment,
    domains: deploymentDomains,
  }
})
