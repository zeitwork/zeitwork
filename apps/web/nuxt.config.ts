import tailwindcss from "@tailwindcss/vite"

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: false },
  css: ["~/assets/css/app.css"],

  app: {
    head: head(),
  },

  compatibilityDate: "2025-07-01",
  future: { compatibilityVersion: 4 },
  fonts: { experimental: { processCSSVariables: true, disableLocalFallbacks: true } },

  // routeRules: {
  //   "/": {
  //     ssr: true,
  //   },
  //   "/auth/github/callback": {
  //     ssr: false,
  //   },
  // },
  ssr: false,

  vite: { plugins: [tailwindcss()] },

  nitro: {
    preset: "bun",
  },

  runtimeConfig: {
    appUrl: process.env.NUXT_APP_URL || "http://localhost:3000",

    public: {
      graphEndpoint: process.env.NUXT_PUBLIC_GRAPH_ENDPOINT,
      githubClientId: process.env.NUXT_PUBLIC_GITHUB_CLIENT_ID || "",
    },

    oauth: {
      github: {
        clientId: process.env.NUXT_PUBLIC_GITHUB_CLIENT_ID || "",
        clientSecret: Buffer.from(process.env.NUXT_GITHUB_CLIENT_SECRET ?? "", "base64").toString("utf-8") ?? "",
        redirectURL: process.env.NUXT_OAUTH_GITHUB_REDIRECT_URL || "http://localhost:3000/auth/github",
      },
    },

    githubAppId: process.env.NUXT_GITHUB_APP_ID || "",
    githubAppPrivateKey: process.env.NUXT_GITHUB_APP_PRIVATE_KEY || "",

    kubeConfig: process.env.NUXT_KUBE_CONFIG || "",
  },

  modules: ["@nuxt/icon", "@nuxt/fonts", "nuxt-auth-utils"],
})

function head() {
  return {
    title: "Zeitwork",
    link: [
      {
        rel: "icon",
        type: "image/x-icon",
        href: "/favicon.png",
        media: "(prefers-color-scheme: dark)",
      },
      {
        rel: "icon",
        type: "image/x-icon",
        href: "/favicon-dark.png",
        media: "(prefers-color-scheme: light)",
      },
    ],
    meta: [
      { charset: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      {
        name: "description",
        content:
          "Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it.",
      },

      // OpenGraph
      { property: "og:title", content: "Zeitword" },
      {
        property: "og:description",
        content:
          "Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it.",
      },
      { property: "og:type", content: "website" },
      { property: "og:url", content: "https://zeitwork.com" },
      { property: "og:image", content: "https://zeitwork.com/og-image.png" },

      // Twitter
      { name: "twitter:card", content: "summary_large_image" },
      { name: "twitter:title", content: "Zeitwork – The fastest way to deploy and scale any application" },
      {
        name: "twitter:description",
        content:
          "Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it.",
      },
      { name: "twitter:image", content: "https://zeitwork.com/og-image.png" },
      { name: "twitter:site", content: "@zeitwork" },
    ],
  }
}
