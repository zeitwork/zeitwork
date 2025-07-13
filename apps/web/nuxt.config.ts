import tailwindcss from "@tailwindcss/vite"

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: false },
  css: ["~/assets/css/app.css"],

  app: {
    head: {
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
            "Zeitwork is a Platform-as-a-Service that automatically builds and deploys your applications from GitHub. Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it. Fully hosted, zero configuration, open source.",
        },

        // OpenGraph
        { property: "og:title", content: "Zeitword" },
        {
          property: "og:description",
          content:
            "Zeitwork is a Platform-as-a-Service that automatically builds and deploys your applications from GitHub. Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it. Fully hosted, zero configuration, open source.",
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
            "Zeitwork is a Platform-as-a-Service that automatically builds and deploys your applications from GitHub. Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it. Fully hosted, zero configuration, open source.",
        },
        { name: "twitter:image", content: "https://zeitwork.com/og-image.png" },
        { name: "twitter:site", content: "@zeitwork" },
      ],
    },
  },

  compatibilityDate: "2025-07-01",
  future: { compatibilityVersion: 4 },
  fonts: { experimental: { processCSSVariables: true, disableLocalFallbacks: true } },

  routeRules: {
    "/": {
      ssr: true,
    },
    "/**": {
      ssr: false,
    },
  },

  vite: { plugins: [tailwindcss()] },

  runtimeConfig: {
    appUrl: "http://localhost:3000",
  },

  modules: ["@nuxt/fonts"],
})
