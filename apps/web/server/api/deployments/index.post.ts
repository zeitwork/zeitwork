import { projectEnvironments, projects } from "@zeitwork/database/schema"
import { eq } from "@zeitwork/database/utils/drizzle"
import { z } from "zod"
import { useDeploymentModel } from "~~/server/models/deployment"

const bodySchema = z.object({
  projectSlug: z.string(),
})

export default defineEventHandler(async (event) => {
  try {
    const timeoutPromise = new Promise((_, reject) =>
      setTimeout(() => reject(new Error("Request timeout after 10 seconds")), 10000),
    )

    const mainLogic = (async () => {
      const { secure } = await requireUserSession(event)
      if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

      // Require active subscription to create deployments
      await requireSubscription(event)

      // Get the project
      const { projectSlug } = await readValidatedBody(event, bodySchema.parse)

      const [project] = await useDrizzle()
        .select()
        .from(projects)
        .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
        .limit(1)

      if (!project) {
        throw createError({ statusCode: 404, message: "Project not found" })
      }

      const [productionEnvironment] = await useDrizzle()
        .select()
        .from(projectEnvironments)
        .where(and(eq(projectEnvironments.projectId, project.id), eq(projectEnvironments.name, "production")))
        .limit(1)

      if (!productionEnvironment) {
        throw createError({ statusCode: 404, message: "Production environment not found" })
      }

      // Create a deployment
      const deploymentModel = useDeploymentModel()
      const { data: deployment, error: deploymentError } = await deploymentModel.create({
        projectId: project.id,
        environmentId: productionEnvironment.id,
        organisationId: secure.organisationId,
      })

      if (deploymentError) {
        throw createError({ statusCode: 500, message: deploymentError.message })
      }

      return deployment
    })()

    return await Promise.race([mainLogic, timeoutPromise])
  } catch (error: any) {
    throw createError({
      statusCode: error.statusCode || 504,
      message: error.message || "Internal server error",
    })
  }
})
