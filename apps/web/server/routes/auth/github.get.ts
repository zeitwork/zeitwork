export default defineOAuthGitHubEventHandler({
  config: {
    emailRequired: true,
  },
  async onSuccess(event, { user, tokens }) {
    console.log("GitHub OAuth success:", user, tokens)

    // Read the code
    const code = getRouterParam(event, "code")
    console.log("code", code)

    const config = useRuntimeConfig()

    const { userId, accessToken } = await $fetch<{
      userId: string
      accessToken: string
    }>("http://localhost:8080/admin/auth/jwt", {
      method: "POST",
      headers: {
        "X-API-KEY": config.apiKey,
      },
      body: {
        username: user.login,
        githubId: user.id,
      },
    })

    console.log("accessToken", accessToken)

    await setUserSession(event, {
      user: {
        id: user.id, // backend user id?
        name: user.name,
        email: user.email,
        username: user.login,
        githubId: user.id,
        avatarUrl: user.avatar_url,
        accessToken,
      },
    })

    return sendRedirect(event, `/${user.login}`)
  },
  // Optional, will return a json error and 401 status code by default
  onError(event, error) {
    console.error("GitHub OAuth error:", error)
    return sendRedirect(event, "/login")
  },
})
