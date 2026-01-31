import { projectEnvironments, projects } from "@zeitwork/database/schema";
import { and, eq } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const project = await findProjectBySlugOrId(id, secure.organisationId!);
  if (!project) throw createError({ statusCode: 404, message: "Project not found" });

  const environments = await findProjectEnvironments(project.id, secure.organisationId!);

  return environments;
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

async function findProjectEnvironments(projectId: string, organisationId: string) {
  const environments = await useDrizzle()
    .select()
    .from(projectEnvironments)
    .where(
      and(
        eq(projectEnvironments.projectId, projectId),
        eq(projectEnvironments.organisationId, organisationId),
      ),
    );
  return environments;
}
