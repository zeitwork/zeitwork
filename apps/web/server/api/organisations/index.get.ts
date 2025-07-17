import { useZeitworkClient } from "../../utils/api"

export default defineEventHandler(async (event) => {
  const { user, secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  if (!user.userId) {
    throw createError({ statusCode: 401, message: "User ID not found in session" })
  }

  const { data, error } = await useZeitworkClient().organisations.list({
    userId: user.userId,
  })

  if (error) {
    throw createError({ statusCode: 500, message: error.message })
  }

  return data
})
