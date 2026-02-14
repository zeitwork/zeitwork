import { isBefore, subMinutes } from "date-fns";

export default defineNitroPlugin(() => {
  sessionHooks.hook("fetch", async (session, _event) => {
    const secure = session.secure;

    // No session or no tokens — nothing to refresh
    if (!secure?.tokens?.refresh_token || !secure?.tokenExpiresAt) {
      return;
    }

    // Token still valid (with 5-minute buffer)
    const expiresAt = new Date(secure.tokenExpiresAt);
    if (isBefore(new Date(), subMinutes(expiresAt, 5))) {
      return;
    }

    console.log("[Session] Access token expired or expiring soon, attempting refresh...");

    try {
      const { app } = useGitHub();

      const { authentication } = await app.oauth.refreshToken({
        refreshToken: secure.tokens.refresh_token,
      });

      if (!authentication.token) {
        throw new Error("No access_token in refresh response");
      }

      // Update session with refreshed tokens
      secure.tokens.access_token = authentication.token;
      secure.tokens.refresh_token = authentication.refreshToken;
      secure.tokenExpiresAt = new Date(authentication.expiresAt).getTime();

      console.log("[Session] Token refreshed successfully");
    } catch (error) {
      console.error("[Session] Token refresh failed, clearing session:", error);

      // Clear the session so the client sees loggedIn = false
      // and the existing auth middleware redirects to /login
      delete session.user;
      delete session.secure;
    }
  });
});
