import { organisationMembers, organisations } from "@zeitwork/database/schema";
import { eq, inArray } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const memberships = await useDrizzle()
    .select()
    .from(organisationMembers)
    .where(eq(organisationMembers.userId, secure.userId));

  const orgs = await useDrizzle()
    .select()
    .from(organisations)
    .where(
      inArray(
        organisations.id,
        memberships.map((el) => el.id),
      ),
    );

  return orgs;
});
