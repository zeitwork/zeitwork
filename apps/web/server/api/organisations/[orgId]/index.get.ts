import { z } from "zod"

const paramsSchema = z.object({
  orgId: z.string().uuid(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId } = await getValidatedRouterParams(event, paramsSchema.parse)

  const { data, error } = await useZeitworkClient().organisations.get({
    organisationId: orgId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
