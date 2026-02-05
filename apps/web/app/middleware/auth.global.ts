export default defineNuxtRouteMiddleware(async (to, from) => {
  if (to.path === "/login") {
    return;
  }

  const { loggedIn, user } = useUserSession();

  if (!loggedIn.value || !user.value?.username) {
    return navigateTo("/login");
  }

  // if logged in, and on / then redirect to the orgs page
  if (to.path === "/" && loggedIn.value) {
    return navigateTo(`/${user.value?.username}`);
  }
});
