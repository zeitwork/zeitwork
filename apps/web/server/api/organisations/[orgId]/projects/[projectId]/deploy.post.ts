import { z } from "zod"
import { useZeitworkClient } from "../../../../../utils/api"
import { getRepository, getLatestCommitSHA } from "../../../../../utils/github"

const paramsSchema = z.object({
  orgId: z.string(),
  projectId: z.string(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId, projectId } = await getValidatedRouterParams(event, paramsSchema.parse)

  const client = useZeitworkClient()

  // First, get the organisation to retrieve its numeric 'no' field and installation ID
  const { data: org, error: orgError } = await client.organisations.get({
    organisationIdOrSlug: orgId,
    userId: user.id,
  })

  if (orgError || !org) {
    console.error(orgError)
    throw createError({ statusCode: 404, message: "Organisation not found" })
  }

  if (!org.installationId) {
    throw createError({ statusCode: 400, message: "GitHub installation not configured for this organisation" })
  }

  // Get the project details
  const { data: project, error: projectError } = await client.projects.get({
    organisationId: orgId,
    organisationNo: org.no,
    projectId,
  })

  if (projectError || !project) {
    throw createError({ statusCode: 404, message: projectError?.message || "Project not found" })
  }

  try {
    // Get repository info from GitHub
    const repoInfo = await getRepository(org.installationId, project.githubOwner, project.githubRepo)

    // Get the latest commit SHA from the default branch
    const latestCommitSHA = await getLatestCommitSHA(
      org.installationId,
      project.githubOwner,
      project.githubRepo,
      repoInfo.defaultBranch,
    )

    console.log(`Deploying latest commit ${latestCommitSHA} for project ${projectId}`)

    // Deploy the project with the latest commit SHA
    const { data: deploymentResult, error: deployError } = await client.projects.deploy({
      organisationId: orgId,
      organisationNo: org.no,
      projectId,
      commitSHA: latestCommitSHA,
    })

    if (deployError || !deploymentResult) {
      throw createError({ statusCode: 500, message: deployError?.message || "Failed to deploy project" })
    }

    return {
      success: true,
      deploymentId: deploymentResult.deploymentId,
      commitSHA: latestCommitSHA,
    }
  } catch (error: any) {
    console.error("Failed to deploy latest commit:", error)
    throw createError({
      statusCode: 500,
      message: error.message || "Failed to deploy latest commit",
    })
  }
})
