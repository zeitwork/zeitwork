import { deployments } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const deploymentId = getRouterParam(event, "id");
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" });
  }

  // Get the deployment with domains
  const [deployment] = await useDrizzle()
    .select()
    .from(deployments)
    .where(
      and(eq(deployments.id, deploymentId), eq(deployments.organisationId, secure.organisationId)),
    );
  if (!deployment) {
    throw createError({ statusCode: 404, message: "Deployment not found" });
  }

  return deployment;
});
