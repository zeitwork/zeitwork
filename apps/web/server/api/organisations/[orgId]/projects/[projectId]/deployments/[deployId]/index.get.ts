import { useZeitworkClient } from "../../../../../../../utils/api"
import { z } from "zod"

const paramsSchema = z.object({
  orgId: z.string().uuid(),
  projectId: z.string(),
  deployId: z.string(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId, deployId } = await getValidatedRouterParams(event, paramsSchema.parse)

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

  const { data, error } = await client.deployments.get({
    projectId,
    deploymentId: deployId,
    organisationId: orgId,
    organisationNo: org.no,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
