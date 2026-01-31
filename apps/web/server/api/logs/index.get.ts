import { buildLogs, deployments, projects } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const querySchema = z.object({
  projectSlug: z.string(),
  deploymentId: z.uuid()
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { projectSlug, deploymentId } = await getValidatedQuery(event, querySchema.parse)

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, projectSlug), eq(projects.organisationId, secure.organisationId)))
    .limit(1)
  if (!project) {
    throw createError({
      statusCode: 404,
      message: "Project not found",
    });
  }

  const [deployment] = await useDrizzle()
    .select()
    .from(deployments)
    .where(and(eq(deployments.id, deploymentId), eq(deployments.organisationId, secure.organisationId)))
    .limit(1)
  if (!deployment) {
    throw createError({
      statusCode: 404,
      message: "Project not found",
    });
  }

  if (!deployment.buildId) {
    return []
  }

  const buildLogList = await useDrizzle()
    .select()
    .from(buildLogs)
    .where(
      and(
        eq(buildLogs.buildId, deployment.buildId),
        eq(buildLogs.organisationId, secure.organisationId),
      ),
    )
    .orderBy(desc(buildLogs.id));

  return buildLogList;
});
