import { projects } from "../../../../../packages/database/schema"
import { eq } from "drizzle-orm"

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const projectList = await useDrizzle()
    .select()
    .from(projects)
    .where(eq(projects.organisationId, secure.organisationId))

  return projectList
})
