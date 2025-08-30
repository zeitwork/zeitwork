import { deployments, projects } from "@zeitwork/database/schema"
import { eq, and } from "drizzle-orm"
import { z } from "zod"

const paramsSchema = z.object({
  id: z.string(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse)

  type Project = typeof projects.$inferSelect & {
    latestDeployment?: typeof deployments.$inferSelect | null
  }

  let project: Project | null = null

  // Is the id a uuid or a slug?
  if (isUUID(id)) {
    let [foundProject] = await useDrizzle()
      .select()
      .from(projects)
      .where(and(eq(projects.id, id), eq(projects.organisationId, secure.organisationId)))
      .limit(1)
    project = foundProject
  } else {
    let [foundProject] = await useDrizzle()
      .select()
      .from(projects)
      .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
      .limit(1)
    project = foundProject
  }

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" })
  }

  // Has deployment, then get the latest deployment
  if (project.latestDeploymentId) {
    let [latestDeployment] = await useDrizzle()
      .select()
      .from(deployments)
      .where(eq(deployments.id, project.latestDeploymentId))
      .limit(1)
    project.latestDeployment = latestDeployment
  } else {
    project.latestDeployment = null
  }

  return project
})

function isUUID(id: string): boolean {
  if (z.string().uuid().safeParse(id).success) {
    return true
  }
  return false
}
