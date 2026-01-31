import { deployments } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const result = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.organisationId, secure.organisationId));

  return result;
});
