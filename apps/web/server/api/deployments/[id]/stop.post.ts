import { deployments } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { deploymentStatus } from "~~/server/models/deployment";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const deploymentId = getRouterParam(event, "id");
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" });
  }

  // Get the deployment to ensure it exists and we have access
  const [deployment] = await useDrizzle()
    .select()
    .from(deployments)
    .where(
      and(eq(deployments.id, deploymentId), eq(deployments.organisationId, secure.organisationId)),
    );
  if (!deployment) {
    throw createError({ statusCode: 404, message: "Deployment not found" });
  }

  const currentStatus = deploymentStatus(deployment);
  if (currentStatus === "stopped" || currentStatus === "failed") {
    throw createError({ statusCode: 400, message: `Deployment is already in a terminal state: ${currentStatus}` });
  }

  // Update the deployment to be stopped
  const [updatedDeployment] = await useDrizzle()
    .update(deployments)
    .set({
      status: "stopped",
      stoppedAt: new Date(),
    })
    .where(
      and(eq(deployments.id, deploymentId), eq(deployments.organisationId, secure.organisationId)),
    )
    .returning();

  return {
    ...updatedDeployment,
    status: deploymentStatus(updatedDeployment),
  };
});
