import { domains, projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
  domainId: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id, domainId } = await getValidatedRouterParams(event, paramsSchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  const [domain] = await useDrizzle()
    .select()
    .from(domains)
    .where(
      and(
        eq(domains.id, domainId),
        eq(domains.projectId, project.id),
        eq(domains.organisationId, secure.organisationId),
      ),
    );

  if (!domain) {
    throw createError({ statusCode: 404, message: "Domain not found" });
  }

  return { verified: !!domain.verifiedAt };
});
