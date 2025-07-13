import tailwindcss from "@tailwindcss/vite"

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: false },
  css: ["~/assets/css/app.css"],

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
