import { z } from "zod"
import { useZeitworkClient } from "../../../../../utils/api"

const paramsSchema = z.object({
  orgId: z.string(),
  projectId: z.string(),
})

const bodySchema = z.object({
  domain: z.string().optional(),
  env: z
    .array(
      z.object({
        name: z.string(),
        value: z.string(),
      }),
    )
    .optional(),
  basePath: z.string().optional(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId } = await getValidatedRouterParams(event, paramsSchema.parse)
  const body = await readValidatedBody(event, bodySchema.parse)

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

  // Update the project
  const { data: updatedProject, error: updateError } = await client.projects.update({
    organisationId: orgId,
    organisationNo: org.no,
    projectId,
    domain: body.domain,
    env: body.env,
    basePath: body.basePath,
  })

  if (updateError || !updatedProject) {
    throw createError({ statusCode: 500, message: updateError?.message || "Failed to update project" })
  }

  return updatedProject
})
