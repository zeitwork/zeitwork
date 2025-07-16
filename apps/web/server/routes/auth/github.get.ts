import { cacheExchange, Client, fetchExchange } from "@urql/core"
import { graphql } from "graphql"
import * as jose from "jose"

const client = new Client({
  url: useRuntimeConfig().public.graphEndpoint,
  exchanges: [fetchExchange],
})

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

    const token = await signJWT(user.id.toString(), config.jwt.secret)

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

async function signJWT(githubUserId: string, secret: string) {
  const jwt = new jose.SignJWT({
    githubId: githubUserId,
  })
    .setProtectedHeader({ alg: "HS256" })
    .setExpirationTime("24h")
    .setIssuedAt()
    .setNotBefore(0)

  return await jwt.sign(new TextEncoder().encode(secret))
}
