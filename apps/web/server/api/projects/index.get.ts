import { projects } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const projectList = await useDrizzle()
    .select()
    .from(projects)
    .where(eq(projects.organisationId, secure.organisationId));

  return projectList;
});
