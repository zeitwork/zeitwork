import { domains, projects, environmentDomains } from "@zeitwork/database/schema"
import { and, eq } from "drizzle-orm"
import { z } from "zod"

const paramsSchema = z.object({
  id: z.string(),
})

const bodySchema = z.object({
  name: z.string().min(1).max(255),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { id: idOrSlug } = await getValidatedRouterParams(event, paramsSchema.parse)
  const { name } = await readValidatedBody(event, bodySchema.parse)

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

  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" })
  }

  const result = await useDrizzle().transaction(async (tx) => {
    // Find or create domain within the same organisation
    let [existingDomain] = await tx
      .select()
      .from(domains)
      .where(and(eq(domains.name, name), eq(domains.organisationId, secure.organisationId)))
      .limit(1)

    if (!existingDomain) {
      const [created] = await tx
        .insert(domains)
        .values({
          name,
          organisationId: secure.organisationId,
        })
        .returning()
      existingDomain = created
    }

    // Ensure project-domain link exists
    const [existingLink] = await tx
      .select()
      .from(environmentDomains)
      .where(
        and(
          eq(environmentDomains.projectId, project.id),
          eq(environmentDomains.domainId, existingDomain.id),
          eq(environmentDomains.organisationId, secure.organisationId),
        ),
      )
      .limit(1)

    if (!existingLink) {
      await tx.insert(environmentDomains).values({
        projectId: project.id,
        domainId: existingDomain.id,
        environmentId: project.productionEnv.id, // TODO: Get the production environment
        organisationId: secure.organisationId,
      })
    }

    return existingDomain
  })

  return result
})
