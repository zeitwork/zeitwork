import { projectEnvironments, projects } from "@zeitwork/database/schema";
import { and, eq } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
  envId: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id, envId } = await getValidatedRouterParams(event, paramsSchema.parse);

  const project = await findProjectBySlugOrId(id, secure.organisationId!);
  if (!project) throw createError({ statusCode: 404, message: "Project not found" });

  const environment = await findEnvironmentByIdOrName(envId, project.id, secure.organisationId!);
  if (!environment) throw createError({ statusCode: 404, message: "Environment not found" });

  return environment;
});

async function findProjectBySlugOrId(id: string, organisationId: string) {
  const isUUID = z.uuid().safeParse(id).success;
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(
      and(
        isUUID ? eq(projects.id, id) : eq(projects.slug, id),
        eq(projects.organisationId, organisationId),
      ),
    )
    .limit(1);
  return project;
}

async function findEnvironmentByIdOrName(envId: string, projectId: string, organisationId: string) {
  const isUUID = z.uuid().safeParse(envId).success;
  const [environment] = await useDrizzle()
    .select()
    .from(projectEnvironments)
    .where(
      and(
        isUUID ? eq(projectEnvironments.id, envId) : eq(projectEnvironments.name, envId),
        eq(projectEnvironments.projectId, projectId),
        eq(projectEnvironments.organisationId, organisationId),
      ),
    )
    .limit(1);
  return environment;
}
