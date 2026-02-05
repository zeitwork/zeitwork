import { domains } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const result = await useDrizzle()
    .select()
    .from(domains)
    .where(and(eq(domains.organisationId, secure.organisationId)))
    .orderBy(desc(domains.id));

  return result;
});
