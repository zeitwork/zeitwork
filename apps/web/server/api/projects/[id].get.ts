import { projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const result = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));

  return result;
});
