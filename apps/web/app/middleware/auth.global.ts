export default defineNuxtRouteMiddleware(async (to, from) => {
  // skip middleware on server
  if (import.meta.server) return

  // if the user is not authenticated, redirect to the login page

  // always allow / and /login
  if (to.path === "/login") {
    return
  }

  const { loggedIn, user } = useUserSession()

  if (!loggedIn.value || !user.value?.username) {
    return navigateTo("/login")
  }

  // if logged in, and on / then redirect to the orgs page
  if (to.path === "/" && loggedIn.value) {
    return navigateTo(`/${user.value?.username}`)
  }

  // Check subscription status (skip for onboarding page itself)
  if (to.path !== "/onboarding") {
    try {
      const { data: subscriptionStatus } = await useFetch("/api/subscription/status")

      if (subscriptionStatus.value && !subscriptionStatus.value.hasSubscription) {
        return navigateTo("/onboarding")
      }
    } catch (error) {
      // If subscription check fails, allow access but log error
      console.error("Failed to check subscription status:", error)
    }
  }
})
