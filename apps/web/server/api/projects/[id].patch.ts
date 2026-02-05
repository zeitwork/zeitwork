import { projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

const bodySchema = z.object({
  rootDirectory: z
    .string()
    .max(255)
    .regex(/^\/(?:[^./][^/]*(?:\/[^./][^/]*)*)?$/, {
      message: "Root directory must start with / and cannot contain '..' or hidden directories",
    })
    .optional(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);
  const body = await readValidatedBody(event, bodySchema.parse);

  // Find the project by slug
  const [existingProject] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .limit(1);

  if (!existingProject) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Build update object with only provided fields
  const updateData: Partial<typeof projects.$inferInsert> = {};
  if (body.rootDirectory !== undefined) {
    updateData.rootDirectory = body.rootDirectory;
  }

  // Only update if there are changes
  if (Object.keys(updateData).length === 0) {
    return { project: existingProject };
  }

  const [updatedProject] = await useDrizzle()
    .update(projects)
    .set({
      ...updateData,
      updatedAt: new Date(),
    })
    .where(eq(projects.id, existingProject.id))
    .returning();

  return { project: updatedProject };
});
