import { environmentVariables, projects } from "@zeitwork/database/schema";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
  envId: z.uuid(),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

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

  // Check if the environment variable exists
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

  // Delete the environment variable
  await useDrizzle()
    .delete(environmentVariables)
    .where(eq(environmentVariables.id, envId));

  return { success: true };
});
