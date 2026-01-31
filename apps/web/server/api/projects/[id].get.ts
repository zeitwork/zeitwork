import { projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.uuid(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const result = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.id, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));

  return result;
});
