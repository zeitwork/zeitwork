import { deployments, projects } from "@zeitwork/database/schema";
import { eq, and, desc, inArray } from "@zeitwork/database/utils/drizzle";

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const db = useDrizzle();

  const projectList = await db
    .select()
    .from(projects)
    .where(eq(projects.organisationId, secure.organisationId));

  const projectIds = projectList.map((p) => p.id);

  // Fetch latest deployment per project
  const latestDeployments =
    projectIds.length > 0
      ? await db
          .selectDistinctOn([deployments.projectId], {
            projectId: deployments.projectId,
            githubCommit: deployments.githubCommit,
            createdAt: deployments.createdAt,
          })
          .from(deployments)
          .where(inArray(deployments.projectId, projectIds))
          .orderBy(deployments.projectId, desc(deployments.createdAt))
      : [];

  return projectList.map((project) => {
    const latest = latestDeployments.find((d) => d.projectId === project.id);
    return {
      ...project,
      latestDeploymentCommit: latest?.githubCommit ?? null,
      latestDeploymentDate: latest?.createdAt ?? null,
    };
  });
});
