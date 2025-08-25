import { z } from "zod"
import { useZeitworkClient } from "../../../../utils/api"
import { organisations } from "~~/packages/database/schema"
import { eq } from "drizzle-orm"
import { useDrizzle } from "../../../../utils/drizzle"

const paramsSchema = z.object({
  orgId: z.string(),
})

const bodySchema = z.object({
  name: z.string().min(1),
  githubOwner: z.string().min(1),
  githubRepo: z.string().min(1),
  port: z.number().min(1).max(65535),
  env: z
    .array(
      z.object({
        name: z.string().min(1),
        value: z.string(),
      }),
    )
    .optional(),
  basePath: z.string().optional(),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId } = await getValidatedRouterParams(event, paramsSchema.parse)
  const { name, githubOwner, githubRepo, port, env, basePath } = await readValidatedBody(event, bodySchema.parse)

  // check if orgId is a uuid or a slug
  const isUuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(orgId)
  let organisation: any
  if (isUuid) {
    ;[organisation] = await useDrizzle().select().from(organisations).where(eq(organisations.id, orgId)).limit(1)
  } else {
    ;[organisation] = await useDrizzle().select().from(organisations).where(eq(organisations.slug, orgId)).limit(1)
  }

  if (!organisation) {
    throw createError({ statusCode: 404, message: "Organisation not found" })
  }

  const { data, error } = await useZeitworkClient().projects.create({
    name,
    githubOwner,
    githubRepo,
    port,
    organisationId: organisation.id,
    env,
    basePath,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
