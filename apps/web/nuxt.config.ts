import tailwindcss from "@tailwindcss/vite";

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: false },
  css: ["~/assets/css/app.css"],

  app: {
    head: head(),
    layoutTransition: { name: "layout-forward", mode: "out-in" },
  },

  compatibilityDate: "latest",
  ssr: false,
  future: { compatibilityVersion: 4 },
  fonts: { experimental: { disableLocalFallbacks: true } },

  vite: { plugins: [tailwindcss()] },

  nitro: {
    preset: "node_server",
  },

  runtimeConfig: {
    appUrl: "http://localhost:3000",

    public: {
      appUrl: "http://localhost:3000",
      appName: "zeitwork",
      githubClientId: "",
      edgeDomain: "edge.zeitwork.com",
      edgeIp: "91.98.13.193",
    },

    oauth: {
      github: {
        clientId: "",
        redirectURL: "http://localhost:3000/auth/github",
      },
    },

    githubAppId: "",
    githubAppPrivateKey: "",

    githubWebhookSecret: "",

    encryptionKey: "",
  },

  modules: ["@nuxt/icon", "@nuxt/fonts", "nuxt-auth-utils", "motion-v/nuxt"],
});

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
      {
        name: "twitter:title",
        content: "Zeitwork - The fastest way to deploy and scale any application",
      },
      {
        name: "twitter:description",
        content:
          "Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork can run it.",
      },
      { name: "twitter:image", content: "https://zeitwork.com/og-image.png" },
      { name: "twitter:site", content: "@zeitwork" },
    ],
  };
}
