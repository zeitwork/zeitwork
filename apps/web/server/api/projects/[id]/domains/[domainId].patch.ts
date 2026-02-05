import { domains, projects } from "@zeitwork/database/schema";
import { eq, and, isNull } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
  domainId: z.string(),
});

const bodySchema = z.object({
  name: z
    .string()
    .min(1, "Domain name is required")
    .regex(
      /^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$/,
      "Invalid domain format",
    ),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id, domainId } = await getValidatedRouterParams(event, paramsSchema.parse);
  const body = await readValidatedBody(event, bodySchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Find existing domain
  const [existingDomain] = await useDrizzle()
    .select()
    .from(domains)
    .where(
      and(
        eq(domains.id, domainId),
        eq(domains.projectId, project.id),
        eq(domains.organisationId, secure.organisationId),
        isNull(domains.deletedAt),
      ),
    );

  if (!existingDomain) {
    throw createError({ statusCode: 404, message: "Domain not found" });
  }

  // If name unchanged, return existing domain as-is
  if (existingDomain.name === body.name) {
    return existingDomain;
  }

  // Check if new name already exists on this project
  const [conflictDomain] = await useDrizzle()
    .select()
    .from(domains)
    .where(and(eq(domains.name, body.name), eq(domains.projectId, project.id)));

  if (conflictDomain && !conflictDomain.deletedAt) {
    throw createError({ statusCode: 400, message: "Domain already exists" });
  }

  // Soft-delete old domain
  await useDrizzle()
    .update(domains)
    .set({ deletedAt: new Date(), updatedAt: new Date() })
    .where(eq(domains.id, existingDomain.id));

  // Resurrect soft-deleted domain or create new
  if (conflictDomain?.deletedAt) {
    const [resurrected] = await useDrizzle()
      .update(domains)
      .set({ deletedAt: null, verifiedAt: null, updatedAt: new Date() })
      .where(eq(domains.id, conflictDomain.id))
      .returning();

    return resurrected;
  }

  const [newDomain] = await useDrizzle()
    .insert(domains)
    .values({
      name: body.name,
      projectId: project.id,
      organisationId: secure.organisationId,
    })
    .returning();

  return newDomain;
});
