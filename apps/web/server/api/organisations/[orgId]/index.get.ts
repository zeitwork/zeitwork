import { z } from "zod"
import { useZeitworkClient } from "../../../utils/api"

const paramsSchema = z.object({
  orgId: z.string(),
})

export default defineEventHandler(async (event) => {
  const session = await requireUserSession(event)
  if (!session.secure?.userId) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" })
  }

  const orgIdOrSlug = getRouterParam(event, "orgId")
  if (!orgIdOrSlug) {
    throw createError({ statusCode: 400, statusMessage: "Organisation ID or slug is required" })
  }

  const client = useZeitworkClient()

  // The client now handles both ID and slug lookups
  const { data, error } = await client.organisations.get({
    organisationIdOrSlug: orgIdOrSlug,
    userId: session.secure.userId,
  })

  if (error) {
    throw createError({ statusCode: 404, statusMessage: error.message })
  }

  return data
})
