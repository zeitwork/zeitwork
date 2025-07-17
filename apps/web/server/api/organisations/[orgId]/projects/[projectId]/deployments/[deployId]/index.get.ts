import { z } from "zod"

const paramsSchema = z.object({
  orgId: z.string().uuid(),
  projectId: z.string().uuid(),
  deployId: z.string().uuid(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId, deployId } = await getValidatedRouterParams(event, paramsSchema.parse)

  const { data, error } = await useZeitworkClient().deployments.get({
    projectId,
    deploymentId: deployId,
    organisationId: orgId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
