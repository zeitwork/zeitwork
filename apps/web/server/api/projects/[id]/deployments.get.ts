import { deployments, projects, domains } from "@zeitwork/database/schema";
import { eq, and, inArray } from "@zeitwork/database/utils/drizzle";
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

  // Step 1: Fetch deployments
  const deploymentList = await useDrizzle()
    .select({
      id: deployments.id,
      status: deployments.status,
      projectId: deployments.projectId,
      githubCommit: deployments.githubCommit,
      organisationId: deployments.organisationId,
      createdAt: deployments.createdAt,
      updatedAt: deployments.updatedAt,
    })
    .from(deployments)
    .where(
      and(
        eq(deployments.projectId, project.id),
        eq(deployments.organisationId, secure.organisationId),
      ),
    )
    .orderBy(desc(deployments.id));

  // Step 2: Fetch domains for these deployments
  const deploymentIds = deploymentList.map((d) => d.id);
  const domainList =
    deploymentIds.length > 0
      ? await useDrizzle()
          .select({
            deploymentId: domains.deploymentId,
            name: domains.name,
          })
          .from(domains)
          .where(inArray(domains.deploymentId, deploymentIds))
      : [];

  // Step 3: Merge domains into deployments
  return deploymentList.map((d) => ({
    ...d,
    domains: domainList.filter((dom) => dom.deploymentId === d.id).map((dom) => dom.name),
  }));
});
