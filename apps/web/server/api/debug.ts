export default defineEventHandler(async (event) => {
  const { user } = await getUserSession(event)
  return {
    user,
  }
})
