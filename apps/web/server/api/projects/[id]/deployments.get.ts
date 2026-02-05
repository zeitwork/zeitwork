import { deployments, projects, domains } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .orderBy(desc(projects.id));
  if (!project) {
    throw createError({
      statusCode: 404,
      message: "Project not found",
    });
  }

  const deploymentList = await useDrizzle()
    .select({
      id: deployments.id,
      status: deployments.status,
      projectId: deployments.projectId,
      githubCommit: deployments.githubCommit,
      organisationId: deployments.organisationId,
      createdAt: deployments.createdAt,
      updatedAt: deployments.updatedAt,
      domain: domains.name,
    })
    .from(deployments)
    .leftJoin(domains, eq(domains.deploymentId, deployments.id))
    .where(
      and(
        eq(deployments.projectId, project.id),
        eq(deployments.organisationId, secure.organisationId),
      ),
    )
    .orderBy(desc(deployments.id));

  return deploymentList;
});
