import { environmentVariables, projects } from "@zeitwork/database/schema";
import { z } from "zod";
import { decrypt } from "~~/server/utils/crypto";

const paramsSchema = z.object({
  id: z.string(),
  envId: z.uuid(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id: projectSlug, envId } = await getValidatedRouterParams(event, paramsSchema.parse);

  // Find the project by slug
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
    .limit(1);

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Get the environment variable
  const [envVar] = await useDrizzle()
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

  if (!envVar) {
    throw createError({ statusCode: 404, message: "Environment variable not found" });
  }

  // Decrypt and return the value
  try {
    const decryptedValue = decrypt(envVar.value);
    return { value: decryptedValue };
  } catch (error) {
    console.error("Failed to decrypt environment variable:", error);
    throw createError({ statusCode: 500, message: "Failed to decrypt value" });
  }
});
