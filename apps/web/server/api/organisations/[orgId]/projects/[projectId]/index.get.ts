import { z } from "zod"
import { useZeitworkClient } from "../../../../../utils/api"

const paramsSchema = z.object({
  orgId: z.string(),
  projectId: z.string(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId } = await getValidatedRouterParams(event, paramsSchema.parse)

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

  const { data: project, error: projectError } = await client.projects.get({
    organisationId: orgId,
    organisationNo: org.no,
    projectId,
  })

  if (projectError || !project) {
    throw createError({ statusCode: 500, message: projectError?.message || "Project not found" })
  }

  // Get deployments for this project
  const { data: deployments, error: deploymentsError } = await client.deployments.list({
    projectId,
    organisationId: orgId,
    organisationNo: org.no,
  })

  // Add the latest deployment URL to the project data
  const latestDeployment = deployments && deployments.length > 0 ? deployments[0] : null
  const projectWithLatestDeployment = {
    ...project,
    latestDeploymentURL: latestDeployment?.previewURL || null,
  }

  return projectWithLatestDeployment
})
