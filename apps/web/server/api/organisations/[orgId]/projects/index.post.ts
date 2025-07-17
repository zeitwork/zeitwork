import { z } from "zod"
import { useZeitworkClient } from "../../../../utils/api"

const paramsSchema = z.object({
  orgId: z.string().uuid(),
})

const bodySchema = z.object({
  name: z.string().min(1),
  githubOwner: z.string().min(1),
  githubRepo: z.string().min(1),
  port: z.number().min(1).max(65535),
})

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { orgId } = await getValidatedRouterParams(event, paramsSchema.parse)
  const { name, githubOwner, githubRepo, port } = await readValidatedBody(event, bodySchema.parse)

  const { data, error } = await useZeitworkClient().projects.create({
    name,
    githubOwner,
    githubRepo,
    port,
    organisationId: orgId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
