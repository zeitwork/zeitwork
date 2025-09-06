import { deployments, domains } from "@zeitwork/database/schema"
import { eq, SQL } from "drizzle-orm"
import { z } from "zod"

const querySchema = z.object({
  projectId: z.string().uuid().optional(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { projectId } = await getValidatedRouterParams(event, querySchema.parse)

  // Get all deployments for the organisation
  let wheres: SQL[] = [eq(deployments.organisationId, secure.organisationId)]

  if (projectId) {
    wheres.push(eq(deployments.projectId, projectId))
  }

  const query = await useDrizzle()
    .select()
    .from(deployments)
    .leftJoin(domains, eq(deployments.id, domains.deploymentId))
    .where(and(...wheres))

  // [{ ..deployment, domains: [..domains] }]
  const result = query.map((row) => ({
    ...row.deployments,
    domains: [row.domains],
  }))

  return result
})
