import { logs, deployments, imageBuilds, instances, deploymentInstances } from "@zeitwork/database/schema"
import { eq, and, or, isNull, isNotNull } from "drizzle-orm"
import { z } from "zod"

const querySchema = z.object({
  context: z.enum(["build", "runtime", "all"]).optional().default("all"),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const deploymentId = getRouterParam(event, "id")
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" })
  }

  const { context: logContext } = await getValidatedQuery(event, querySchema.parse)

  // Get the deployment to verify access
  const [deployment] = await useDrizzle()
    .select()
    .from(deployments)
    .where(
      and(
        eq(deployments.id, deploymentId),
        eq(deployments.organisationId, secure.organisationId)
      )
    )
    .limit(1)

  if (!deployment) {
    throw createError({ statusCode: 404, message: "Deployment not found" })
  }

  // Build the where condition based on context
  let whereCondition

  if (logContext === "build") {
    // Only build logs (has image_build_id)
    const buildLogs = await useDrizzle()
      .select()
      .from(logs)
      .leftJoin(imageBuilds, eq(logs.imageBuildId, imageBuilds.id))
      .where(
        and(
          eq(imageBuilds.id, deployment.imageBuildId!),
          isNotNull(logs.imageBuildId)
        )
      )
      .orderBy(logs.loggedAt)

    return buildLogs.map(row => row.logs)
  } else if (logContext === "runtime") {
    // Only runtime logs (has instance_id)
    const runtimeLogs = await useDrizzle()
      .select({
        log: logs,
      })
      .from(logs)
      .leftJoin(instances, eq(logs.instanceId, instances.id))
      .leftJoin(deploymentInstances, eq(instances.id, deploymentInstances.instanceId))
      .where(
        and(
          eq(deploymentInstances.deploymentId, deployment.id),
          isNotNull(logs.instanceId)
        )
      )
      .orderBy(logs.loggedAt)

    return runtimeLogs.map(row => row.log)
  } else {
    // All logs - both build and runtime
    const allLogs = await useDrizzle()
      .select()
      .from(logs)
      .leftJoin(imageBuilds, eq(logs.imageBuildId, imageBuilds.id))
      .leftJoin(instances, eq(logs.instanceId, instances.id))
      .leftJoin(deploymentInstances, eq(instances.id, deploymentInstances.instanceId))
      .where(
        or(
          and(
            eq(imageBuilds.id, deployment.imageBuildId!),
            isNotNull(logs.imageBuildId)
          ),
          and(
            eq(deploymentInstances.deploymentId, deployment.id),
            isNotNull(logs.instanceId)
          )
        )
      )
      .orderBy(logs.loggedAt)

    return allLogs.map(row => row.logs)
  }
})

