import * as k8s from "@kubernetes/client-node"

export default defineEventHandler(async (event) => {
  try {
    const config = useRuntimeConfig()

    console.log("--------------------------------")
    console.log("Debugging kubeconfig...")

    let debugInfo: any = {
      kubeconfig: {
        exists: !!config.kubeConfig,
        length: config.kubeConfig?.length || 0,
        isJson: false,
        isYaml: false,
        currentContext: null,
        contexts: [],
        clusters: [],
        error: null,
      },
    }

    if (config.kubeConfig) {
      // Check format
      debugInfo.kubeconfig.isJson = config.kubeConfig.trim().startsWith("{")
      debugInfo.kubeconfig.isYaml = config.kubeConfig.includes("apiVersion:") || config.kubeConfig.includes("kind:")

      // Try to parse the kubeconfig
      const kc = new k8s.KubeConfig()
      try {
        kc.loadFromString(config.kubeConfig)

        debugInfo.kubeconfig.currentContext = kc.getCurrentContext() || null
        debugInfo.kubeconfig.contexts = kc.getContexts().map((c) => ({
          name: c.name,
          cluster: c.cluster,
          user: c.user,
        }))
        debugInfo.kubeconfig.clusters = kc.getClusters().map((c) => ({
          name: c.name,
          server: c.server?.replace(/https:\/\/[^\/]+/, "https://[REDACTED]") || null,
        }))
        debugInfo.kubeconfig.users = kc.getUsers().map((u) => ({
          name: u.name,
          hasToken: !!u.token,
          hasCert: !!u.certData,
          hasKey: !!u.keyData,
        }))
      } catch (error) {
        debugInfo.kubeconfig.error = error instanceof Error ? error.message : "Unknown error"
      }
    }

    console.log(JSON.stringify(debugInfo, null, 2))
    console.log("--------------------------------")
  } catch {
    return {}
  }

  return {}
})
