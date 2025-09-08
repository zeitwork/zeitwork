import { deployments, domains, projectDomains, projects } from "@zeitwork/database/schema"
import { eq, inArray, or, SQL } from "drizzle-orm"
import { z } from "zod"

const paramsSchema = z.object({
  id: z.string(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { id: idOrSlug } = await getValidatedRouterParams(event, paramsSchema.parse)

  // Get all deployments for the organisation
  let wheres: SQL[] = [eq(domains.organisationId, secure.organisationId)]

  let domainIds: string[] = []
  if (idOrSlug) {
    const isUuid = z.string().uuid().safeParse(idOrSlug).success

    const [project] = await useDrizzle()
      .select()
      .from(projects)
      .where(
        and(
          ...(isUuid ? [eq(projects.id, idOrSlug)] : [eq(projects.slug, idOrSlug)]),
          eq(projects.organisationId, secure.organisationId),
        ),
      )
      .limit(1)

    const projectDomainList = await useDrizzle()
      .select()
      .from(projectDomains)
      .where(and(eq(projectDomains.projectId, project.id), eq(projectDomains.organisationId, secure.organisationId)))

    if (!projectDomainList) return []

    domainIds = projectDomainList.map((projectDomain) => projectDomain.domainId).filter((domainId) => domainId !== null)
  }

  if (domainIds.length > 0) {
    wheres.push(inArray(domains.id, domainIds))
  } else {
    return []
  }

  const result = await useDrizzle()
    .select()
    .from(domains)
    .where(and(...wheres))
    .orderBy(desc(domains.id))

  return result
})
