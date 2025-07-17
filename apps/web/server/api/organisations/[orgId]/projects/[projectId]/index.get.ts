import { z } from "zod"

const paramsSchema = z.object({
  orgId: z.string().uuid(),
  projectId: z.string().uuid(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId } = await getValidatedRouterParams(event, paramsSchema.parse)

  const { data, error } = await useZeitworkClient().projects.get({
    organisationId: orgId,
    projectId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
