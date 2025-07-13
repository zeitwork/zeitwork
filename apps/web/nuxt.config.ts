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
          href: "/favicon-light.png",
          media: "(prefers-color-scheme: light)",
        },
      ],
    },
  },

  compatibilityDate: "2025-07-01",
  future: { compatibilityVersion: 4 },
  fonts: { experimental: { processCSSVariables: true, disableLocalFallbacks: true } },
  ssr: false,

  vite: { plugins: [tailwindcss()] },

  runtimeConfig: {
    appUrl: "http://localhost:3000",
  },

  app: {
    head: {
      title: "Zeitwork",
    },
  },

  modules: ["@nuxt/fonts"],
})
