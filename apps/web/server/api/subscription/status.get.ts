export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)

  if (!secure?.organisationId) {
    return {
      hasSubscription: false,
      status: null,
    }
  }

  const hasSubscription = await hasValidSubscription(secure.organisationId)

  return {
    hasSubscription,
  }
})
