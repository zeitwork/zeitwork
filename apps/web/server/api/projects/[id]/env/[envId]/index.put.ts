import { environmentVariables, projects } from "@zeitwork/database/schema";
import { z } from "zod";
import { encrypt } from "~~/server/utils/crypto";

const paramsSchema = z.object({
  id: z.string(),
  envId: z.uuid(),
});

const bodySchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(255, "Name must be 255 characters or less")
    .regex(/^[A-Z_][A-Z0-9_]*$/i, "Name must be a valid environment variable name")
    .optional(),
  value: z.string().optional(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id: projectSlug, envId } = await getValidatedRouterParams(event, paramsSchema.parse);
  const body = await readValidatedBody(event, bodySchema.parse);

  if (!body.name && body.value === undefined) {
    throw createError({ statusCode: 400, message: "At least one of name or value must be provided" });
  }

  // Find the project by slug
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
    .limit(1);

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Get the existing environment variable
  const [existing] = await useDrizzle()
    .select()
    .from(environmentVariables)
    .where(
      and(
        eq(environmentVariables.id, envId),
        eq(environmentVariables.projectId, project.id),
        eq(environmentVariables.organisationId, secure.organisationId),
      ),
    )
    .limit(1);

  if (!existing) {
    throw createError({ statusCode: 404, message: "Environment variable not found" });
  }

  // Check for name conflict if name is being changed
  if (body.name && body.name !== existing.name) {
    const [conflict] = await useDrizzle()
      .select()
      .from(environmentVariables)
      .where(
        and(
          eq(environmentVariables.name, body.name),
          eq(environmentVariables.projectId, project.id),
        ),
      )
      .limit(1);

    if (conflict) {
      throw createError({ statusCode: 409, message: "Environment variable with this name already exists" });
    }
  }

  // Build update object
  const updateData: { name?: string; value?: string; updatedAt: Date } = {
    updatedAt: new Date(),
  };

  if (body.name) {
    updateData.name = body.name;
  }

  if (body.value !== undefined) {
    updateData.value = encrypt(body.value);
  }

  // Update the environment variable
  const [updated] = await useDrizzle()
    .update(environmentVariables)
    .set(updateData)
    .where(eq(environmentVariables.id, envId))
    .returning({
      id: environmentVariables.id,
      name: environmentVariables.name,
      createdAt: environmentVariables.createdAt,
      updatedAt: environmentVariables.updatedAt,
    });

  return updated;
});
