export default defineNuxtRouteMiddleware(async (to, from) => {
  // skip middleware on server
  if (import.meta.server) return;

  // if the user is not authenticated, redirect to the login page

  // always allow / and /login
  if (to.path === "/login") {
    return;
  }

  // always allow /api
  if (to.path.startsWith("/api")) {
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

  // Check subscription status
  try {
    const { session } = useUserSession();
    const isOnboarding = to.path === "/onboarding";
    const justCompletedCheckout = to.query.checkout === "success";

    // If we have a subscribed status in session and we're not on onboarding
    // and didn't just complete checkout, skip API call
    if (session.value?.hasSubscription && !isOnboarding && !justCompletedCheckout) {
      return;
    }

    // Fetch subscription status if:
    // - We're on onboarding (to potentially redirect away if subscribed)
    // - We don't have a subscribed status in session
    // - User just completed checkout (need to refresh their status)
    const subscriptionStatus = await $fetch("/api/subscription/status");

    // If on onboarding page and has subscription, redirect to user org
    if (isOnboarding && subscriptionStatus?.hasSubscription) {
      return navigateTo(`/${user.value.username}`);
    }

    // If not on onboarding page and no subscription, redirect to onboarding
    if (!isOnboarding && subscriptionStatus && !subscriptionStatus.hasSubscription) {
      return navigateTo("/onboarding");
    }
  } catch (error) {
    // If subscription check fails, allow access but log error
    console.error("Failed to check subscription status:", error);
  }
});
