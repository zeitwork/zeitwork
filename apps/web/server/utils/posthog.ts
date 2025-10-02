import { PostHog } from "posthog-node"

let posthogClient: PostHog | null = null

export function usePostHog() {
  if (posthogClient) {
    return posthogClient
  }

  const config = useRuntimeConfig()

  if (!config.public.posthog.publicKey) {
    console.warn("PostHog API key not configured")
    // Return a mock client that does nothing
    return {
      capture: () => Promise.resolve(),
      identify: () => Promise.resolve(),
      shutdown: () => Promise.resolve(),
    } as unknown as PostHog
  }

  posthogClient = new PostHog(config.public.posthog.publicKey, {
    host: config.public.posthog.host,
  })

  return posthogClient
}
