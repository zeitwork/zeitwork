import { deployments, projects, domains } from "@zeitwork/database/schema";
import { eq, and, lt, inArray } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";
import { deploymentStatus } from "~~/server/models/deployment";

const paramsSchema = z.object({
  id: z.string(),
});

const querySchema = z.object({
  cursor: z.string().uuid().optional(),
  limit: z.coerce.number().int().min(1).max(100).default(20),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);
  const { cursor, limit } = await getValidatedQuery(event, querySchema.parse);

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

  // Step 1: Fetch deployments with cursor-based pagination
  const conditions = [
    eq(deployments.projectId, project.id),
    eq(deployments.organisationId, secure.organisationId),
  ];
  if (cursor) {
    conditions.push(lt(deployments.id, cursor));
  }

  const deploymentList = await useDrizzle()
    .select()
    .from(deployments)
    .where(and(...conditions))
    .orderBy(desc(deployments.id))
    .limit(limit);

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
  const merged = deploymentList.map((d) => ({
    ...d,
    status: deploymentStatus(d),
    domains: domainList.filter((dom) => dom.deploymentId === d.id).map((dom) => dom.name),
  }));

  const nextCursor = deploymentList.length === limit ? (deploymentList.at(-1)?.id ?? null) : null;

  return { deployments: merged, nextCursor };
});
