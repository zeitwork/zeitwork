import { defineNuxtPlugin } from "#app"
import posthog from "posthog-js"

export default defineNuxtPlugin((nuxtApp) => {
  const runtimeConfig = useRuntimeConfig()
  const posthogClient = posthog.init(runtimeConfig.public.posthog.publicKey, {
    api_host: runtimeConfig.public.posthog.host,
    defaults: runtimeConfig.public.posthog.defaults,
    person_profiles: "identified_only", // or 'always' to create profiles for anonymous users as well
    loaded: (posthog) => {
      if (import.meta.env.MODE === "development") posthog.debug()
    },
  })

  return {
    provide: {
      posthog: () => posthogClient,
    },
  }
})
