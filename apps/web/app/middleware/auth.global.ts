export default defineNuxtRouteMiddleware(async (to, from) => {
  // Allow login and waitlist pages without auth checks
  if (to.path === "/login" || to.path === "/waitlist" || to.path.startsWith("/ui")) {
    return;
  }

  const { loggedIn, user } = useUserSession();

  if (!loggedIn.value || !user.value?.username) {
    return navigateTo("/login");
  }

  // Redirect unverified users to waitlist
  if (!user.value.verifiedAt) {
    return navigateTo("/waitlist");
  }

  // if logged in, and on / then redirect to the orgs page
  if (to.path === "/" && loggedIn.value) {
    return navigateTo(`/${user.value?.username}`);
  }
});
