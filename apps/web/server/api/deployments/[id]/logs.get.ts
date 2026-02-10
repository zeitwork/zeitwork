import { vmLogs, deployments } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

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

  if (!deployment.vmId) {
    return [];
  }

  // Fetch VM logs via the deployment's vmId
  const logs = await useDrizzle()
    .select()
    .from(vmLogs)
    .where(eq(vmLogs.vmId, deployment.vmId))
    .orderBy(vmLogs.createdAt);

  return logs;
});
