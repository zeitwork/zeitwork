import { environmentVariables, projects } from "@zeitwork/database/schema";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id: projectSlug } = await getValidatedRouterParams(event, paramsSchema.parse);

  // Find the project by slug
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
    .limit(1);

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Get all environment variables for this project (without decrypting values)
  const envVars = await useDrizzle()
    .select({
      id: environmentVariables.id,
      name: environmentVariables.name,
      createdAt: environmentVariables.createdAt,
      updatedAt: environmentVariables.updatedAt,
    })
    .from(environmentVariables)
    .where(
      and(
        eq(environmentVariables.projectId, project.id),
        eq(environmentVariables.organisationId, secure.organisationId),
      ),
    )
    .orderBy(environmentVariables.name);

  return envVars;
});
