import { domains, projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));
  if (!project) {
    throw createError({
      statusCode: 404,
      message: "Project not found",
    });
  }

  const domainList = await useDrizzle()
    .select()
    .from(domains)
    .where(
      and(eq(domains.projectId, project.id), eq(domains.organisationId, secure.organisationId)),
    )
    .orderBy(desc(domains.id));

  // Filter out internal zeitwork.app domains
  return domainList.filter((domain) => !domain.name.endsWith(".zeitwork.app"));
});
