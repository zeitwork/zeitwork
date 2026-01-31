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
    appUrl: process.env.NUXT_APP_URL || "http://localhost:3000",

    public: {
      domainTarget: "167.233.9.179",

      appUrl: process.env.NUXT_APP_URL || "http://localhost:3000",
      appName: process.env.NUXT_APP_NAME || "zeitwork",
      graphEndpoint: process.env.NUXT_PUBLIC_GRAPH_ENDPOINT,
      githubClientId: process.env.NUXT_PUBLIC_GITHUB_CLIENT_ID || "",

      stripeEnabled: process.env.NUXT_STRIPE_ENABLED !== "false",

      stripe: {
        planEarlyAccessId: process.env.NUXT_PUBLIC_STRIPE_PLAN_EARLY_ACCESS_ID || "",
        planHobbyId: process.env.NUXT_PUBLIC_STRIPE_PLAN_HOBBY_ID || "",
        planBusinessId: process.env.NUXT_PUBLIC_STRIPE_PLAN_BUSINESS_ID || "",
      },

      posthog: {
        publicKey: process.env.NUXT_PUBLIC_POSTHOG_PUBLIC_KEY || "",
        host: process.env.NUXT_PUBLIC_POSTHOG_HOST || "https://us.i.posthog.com",
        defaults: process.env.NUXT_PUBLIC_POSTHOG_DEFAULTS || "2025-05-24",
      },
    },

    oauth: {
      github: {
        clientId: process.env.NUXT_PUBLIC_GITHUB_CLIENT_ID || "",
        redirectURL:
          process.env.NUXT_OAUTH_GITHUB_REDIRECT_URL || "http://localhost:3000/auth/github",
      },
    },

    githubAppId: process.env.NUXT_GITHUB_APP_ID || "",
    githubAppPrivateKey: process.env.NUXT_GITHUB_APP_PRIVATE_KEY || "",

    githubWebhookSecret: "",

    kubeConfig: process.env.NUXT_KUBE_CONFIG || "",

    stripe: {
      secretKey: process.env.NUXT_STRIPE_SECRET_KEY || "",
      webhookSecret: process.env.NUXT_STRIPE_WEBHOOK_SECRET || "",
    },
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
