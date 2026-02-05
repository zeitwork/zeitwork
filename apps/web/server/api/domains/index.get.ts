import { domains } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const result = await useDrizzle()
    .select()
    .from(domains)
    .where(and(eq(domains.organisationId, secure.organisationId)))
    .orderBy(desc(domains.id));

  return result;
});
