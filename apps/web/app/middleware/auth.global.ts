export default defineNuxtRouteMiddleware(async (to, from) => {
  // skip middleware on server
  if (import.meta.server) return

  // if the user is not authenticated, redirect to the login page

  // always allow / and /login
  if (to.path === "/login" || to.path === "/") {
    return
  }

  const { loggedIn } = useUserSession()

  if (!loggedIn.value) {
    return navigateTo("/login")
  }
})
