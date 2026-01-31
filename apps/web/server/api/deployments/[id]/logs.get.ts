import { deploymentLogs, deployments } from "@zeitwork/database/schema";
import { eq, and, desc } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const deploymentId = getRouterParam(event, "id");
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" });
  }

  // Get the deployment to verify access
  const [deployment] = await useDrizzle()
    .select()
    .from(deployments)
    .where(
      and(eq(deployments.id, deploymentId), eq(deployments.organisationId, secure.organisationId)),
    )
    .limit(1);

  if (!deployment) {
    throw createError({ statusCode: 404, message: "Deployment not found" });
  }

  // Fetch all deployment logs
  const logs = await useDrizzle()
    .select()
    .from(deploymentLogs)
    .where(eq(deploymentLogs.deploymentId, deployment.id))
    .orderBy(deploymentLogs.createdAt);

  return logs;
});
