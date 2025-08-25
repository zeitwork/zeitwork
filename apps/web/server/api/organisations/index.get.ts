import { useZeitworkClient } from "../../utils/api"

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const { data, error } = await useZeitworkClient().organisations.list({
    userId: secure.userId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
