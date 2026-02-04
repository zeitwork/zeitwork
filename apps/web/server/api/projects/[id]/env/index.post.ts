import { environmentVariables, projects } from "@zeitwork/database/schema";
import { z } from "zod";
import { encrypt } from "~~/server/utils/crypto";

const paramsSchema = z.object({
  id: z.string(),
});

const bodySchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(255, "Name must be 255 characters or less")
    .regex(/^[A-Z_][A-Z0-9_]*$/i, "Name must be a valid environment variable name"),
  value: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id: projectSlug } = await getValidatedRouterParams(event, paramsSchema.parse);
  const body = await readValidatedBody(event, bodySchema.parse);

  // Find the project by slug
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
    .limit(1);

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Check if env var with same name already exists
  const [existing] = await useDrizzle()
    .select()
    .from(environmentVariables)
    .where(
      and(
        eq(environmentVariables.name, body.name),
        eq(environmentVariables.projectId, project.id),
      ),
    )
    .limit(1);

  if (existing) {
    throw createError({ statusCode: 409, message: "Environment variable with this name already exists" });
  }

  // Encrypt the value and insert
  const encryptedValue = encrypt(body.value);

  const [envVar] = await useDrizzle()
    .insert(environmentVariables)
    .values({
      name: body.name,
      value: encryptedValue,
      projectId: project.id,
      organisationId: secure.organisationId,
    })
    .returning({
      id: environmentVariables.id,
      name: environmentVariables.name,
      createdAt: environmentVariables.createdAt,
      updatedAt: environmentVariables.updatedAt,
    });

  return envVar;
});
