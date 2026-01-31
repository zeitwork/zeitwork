import { deployments, domains, projects, environmentDomains } from "@zeitwork/database/schema";
import { eq, and, inArray } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  type LatestDeployment = typeof deployments.$inferSelect & {
    domains?: (typeof domains.$inferSelect)[];
  };

  type ProjectDomain = typeof environmentDomains.$inferSelect & {
    domain?: typeof domains.$inferSelect | null;
  };

  type Project = typeof projects.$inferSelect & {
    latestDeployment?: LatestDeployment | null;
    domains?: ProjectDomain[];
  };

  let project: Project | null = null;

  // Is the id a uuid or a slug?
  if (isUUID(id)) {
    let [foundProject] = await useDrizzle()
      .select()
      .from(projects)
      .where(and(eq(projects.id, id), eq(projects.organisationId, secure.organisationId)))
      .limit(1);
    project = foundProject;
  } else {
    let [foundProject] = await useDrizzle()
      .select()
      .from(projects)
      .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
      .limit(1);
    project = foundProject;
  }

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // project domains
  const projectDomainList = await useDrizzle()
    .select()
    .from(environmentDomains)
    .where(eq(environmentDomains.projectId, project.id));
  project.domains = projectDomainList;

  return project;
});

function isUUID(id: string): boolean {
  if (z.string().uuid().safeParse(id).success) {
    return true;
  }
  return false;
}
