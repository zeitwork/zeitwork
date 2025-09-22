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
})
