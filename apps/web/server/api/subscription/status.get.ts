export default defineEventHandler(async (event) => {
  const session = await requireUserSession(event)

  if (!session.secure?.organisationId) {
    // Update session with no subscription status
    await setUserSession(event, {
      ...session,
      hasSubscription: false,
      subscriptionCheckedAt: Date.now(),
    })

    return {
      hasSubscription: false,
      status: null,
    }
  }

  const hasSubscription = await hasValidSubscription(session.secure.organisationId)

  // Cache the subscription status in the session
  await setUserSession(event, {
    ...session,
    hasSubscription,
    subscriptionCheckedAt: Date.now(),
  })

  return {
    hasSubscription,
  }
})
