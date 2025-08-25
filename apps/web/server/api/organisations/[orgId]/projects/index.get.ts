import { z } from "zod"
import { useZeitworkClient } from "../../../../utils/api"

const paramsSchema = z.object({
  orgId: z.string(),
})

export default defineEventHandler(async (event) => {
  const { secure, user } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId } = await getValidatedRouterParams(event, paramsSchema.parse)

  const client = useZeitworkClient()

  // First, get the organisation to retrieve its numeric 'no' field
  const { data: org, error: orgError } = await client.organisations.get({
    organisationIdOrSlug: orgId,
    userId: user.id,
  })

  if (orgError || !org) {
    console.error(orgError)
    throw createError({ statusCode: 404, message: "Organisation not found" })
  }

  const { data, error } = await client.projects.list({
    organisationId: orgId,
    organisationNo: org.no,
  })

  if (error) {
    console.error(error)
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
