import { vmLogs, deployments } from "@zeitwork/database/schema";
import { eq, and, gt } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const querySchema = z.object({
  cursor: z.uuid().optional(),
  limit: z.coerce.number().int().min(1).max(1000).default(200),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const deploymentId = getRouterParam(event, "id");
  if (!deploymentId) {
    throw createError({ statusCode: 400, message: "Deployment ID is required" });
  }

  const { cursor, limit } = await getValidatedQuery(event, querySchema.parse);

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
    return { logs: [], nextCursor: null };
  }

  // Build conditions with optional cursor
  const conditions = [eq(vmLogs.vmId, deployment.vmId)];
  if (cursor) {
    conditions.push(gt(vmLogs.id, cursor));
  }

  // Fetch VM logs via the deployment's vmId
  const logs = await useDrizzle()
    .select()
    .from(vmLogs)
    .where(and(...conditions))
    .orderBy(vmLogs.id)
    .limit(limit);

  const nextCursor = logs.length === limit ? (logs.at(-1)?.id ?? null) : null;

  return { logs, nextCursor };
});
