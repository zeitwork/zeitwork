import { z } from "zod"
import { useZeitworkClient } from "../../../utils/api"
import { useDrizzle, eq, and } from "../../../utils/drizzle"
import * as schema from "@zeitwork/database/schema"

const paramsSchema = z.object({
  orgId: z.string(),
})

export default defineEventHandler(async (event) => {
  const session = await requireUserSession(event)
  if (!session.secure?.userId) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" })
  }

  const orgIdOrSlug = getRouterParam(event, "orgId")
  if (!orgIdOrSlug) {
    throw createError({ statusCode: 400, statusMessage: "Organisation ID or slug is required" })
  }

  const client = useZeitworkClient()

  // First try to fetch by ID (UUID format)
  const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
  const isUuid = uuidRegex.test(orgIdOrSlug)

  if (isUuid) {
    const { data, error } = await client.organisations.get({
      organisationId: orgIdOrSlug,
      userId: session.secure.userId,
    })

    if (error) {
      throw createError({ statusCode: 404, statusMessage: error.message })
    }

    return data
  } else {
    // If not UUID, treat as slug - need to add this functionality to the client
    const db = useDrizzle()
    const [org] = await db
      .select({
        id: schema.organisations.id,
        name: schema.organisations.name,
        slug: schema.organisations.slug,
        installationId: schema.organisations.installationId,
      })
      .from(schema.organisations)
      .innerJoin(schema.organisationMembers, eq(schema.organisations.id, schema.organisationMembers.organisationId))
      .where(
        and(eq(schema.organisations.slug, orgIdOrSlug), eq(schema.organisationMembers.userId, session.secure.userId)),
      )
      .limit(1)

    if (!org) {
      throw createError({ statusCode: 404, statusMessage: "Organisation not found" })
    }

    return org
  }
})
