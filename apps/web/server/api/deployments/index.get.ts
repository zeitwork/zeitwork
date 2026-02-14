import { deployments } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";
import { deploymentStatus } from "~~/server/models/deployment";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const result = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.organisationId, secure.organisationId));

  return result.map((deployment) => ({
    ...deployment,
    status: deploymentStatus(deployment),
  }));
});
