import {
  domains,
  projects,
  environmentDomains,
  projectEnvironments,
} from "@zeitwork/database/schema";
import { and, eq } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

const bodySchema = z.object({
  name: z.hostname().min(1).max(255),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  // Require active subscription to add domains
  await requireSubscription(event);

  const { id: idOrSlug } = await getValidatedRouterParams(event, paramsSchema.parse);
  const { name } = await readValidatedBody(event, bodySchema.parse);

  const project = await findProjectBySlugOrId(idOrSlug, secure.organisationId!);
  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  const [productionEnv] = await useDrizzle()
    .select()
    .from(projectEnvironments)
    .where(
      and(
        eq(projectEnvironments.projectId, project.id),
        eq(projectEnvironments.name, "production"),
        eq(projectEnvironments.organisationId, secure.organisationId!),
      ),
    )
    .limit(1);

  if (!productionEnv) {
    throw createError({ statusCode: 404, message: "Production environment not found" });
  }

  const result = await useDrizzle().transaction(async (tx) => {
    // Find or create domain within the same organisation
    let [existingDomain] = await tx
      .select()
      .from(domains)
      .where(and(eq(domains.name, name), eq(domains.organisationId, secure.organisationId!)))
      .limit(1);

    if (!existingDomain) {
      const [created] = await tx
        .insert(domains)
        .values({
          name,
          organisationId: secure.organisationId!,
        })
        .returning();
      existingDomain = created;
    }

    // Ensure project-domain link exists
    const [existingLink] = await tx
      .select()
      .from(environmentDomains)
      .where(
        and(
          eq(environmentDomains.projectId, project.id),
          eq(environmentDomains.domainId, existingDomain.id),
          eq(environmentDomains.organisationId, secure.organisationId!),
        ),
      )
      .limit(1);

    if (!existingLink) {
      await tx.insert(environmentDomains).values({
        projectId: project.id,
        domainId: existingDomain.id,
        environmentId: productionEnv.id,
        organisationId: secure.organisationId!,
      });
    }

    return existingDomain;
  });

  return result;
});

async function findProjectBySlugOrId(slugOrId: string, organisationId: string) {
  const isUuid = z.uuid().safeParse(slugOrId).success;

  const findByField = isUuid ? eq(projects.id, slugOrId) : eq(projects.slug, slugOrId);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(findByField, eq(projects.organisationId, organisationId)))
    .limit(1);

  return project;
}
