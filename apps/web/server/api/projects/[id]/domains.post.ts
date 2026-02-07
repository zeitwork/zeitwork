import { domains, projects } from "@zeitwork/database/schema";
import { eq, and, ne, isNull } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
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

/**
 * Check if a domain name is already in use by another active (non-deleted)
 * domain on any project across the platform. If so, TXT verification will
 * be required (Vercel-style ownership proof).
 */
async function isDomainClaimedElsewhere(name: string, excludeProjectId: string): Promise<boolean> {
  const [conflict] = await useDrizzle()
    .select({ id: domains.id })
    .from(domains)
    .where(
      and(
        eq(domains.name, name),
        ne(domains.projectId, excludeProjectId),
        isNull(domains.deletedAt),
      ),
    )
    .limit(1);

  return !!conflict;
}

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);
  const body = await readValidatedBody(event, bodySchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Check if domain already exists on this project
  const [existingDomain] = await useDrizzle()
    .select()
    .from(domains)
    .where(and(eq(domains.name, body.name), eq(domains.projectId, project.id)));

  if (existingDomain) {
    if (!existingDomain.deletedAt) {
      throw createError({ statusCode: 400, message: "Domain already exists" });
    }

    // Resurrect soft-deleted domain — re-check if TXT verification is needed
    const txtRequired = await isDomainClaimedElsewhere(body.name, project.id);

    const [resurrected] = await useDrizzle()
      .update(domains)
      .set({
        deletedAt: null,
        verifiedAt: null,
        txtVerificationRequired: txtRequired,
        updatedAt: new Date(),
      })
      .where(eq(domains.id, existingDomain.id))
      .returning();

    return resurrected;
  }

  // Check if domain is claimed by another project on the platform
  const txtRequired = await isDomainClaimedElsewhere(body.name, project.id);

  const [newDomain] = await useDrizzle()
    .insert(domains)
    .values({
      name: body.name,
      projectId: project.id,
      organisationId: secure.organisationId,
      txtVerificationRequired: txtRequired,
    })
    .returning();

  return newDomain;
});
