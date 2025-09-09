<script setup lang="ts">
import { useTimeoutFn } from "@vueuse/core"
import { ExternalLinkIcon, GitCommitIcon, ClockIcon, CheckCircleIcon } from "lucide-vue-next"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectSlug = route.params.project as string

const { data: deployments, refresh: refreshDeployments } = await useFetch(`/api/deployments`, {
  query: {
    projectSlug,
  },
})

const { isPending, start, stop } = useTimeoutFn(() => {
  refreshDeployments()
}, 1000)

start()

watch(
  deployments,
  (value) => {
    const anyPending = value?.some(
      (deployment) =>
        deployment.status === "pending" || deployment.status === "building" || deployment.status === "deploying",
    )
    if (anyPending) {
      start()
    } else {
      stop()
    }
  },
  { immediate: true },
)

async function createDeployment() {
  await $fetch(`/api/deployments`, {
    method: "POST",
    body: {
      projectSlug,
    },
  })
  await refreshDeployments()
}

function renderDate(date: string) {
  return Intl.DateTimeFormat("en-DE", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(date))
}

function formatDeploymentUrl(deployment: any) {
  return `https://${deployment.domains?.[0].name}:8443`
}

function deploymentStatusColor(status: string) {
  switch (status) {
    case "pending":
      return "text-yellow-500"
    case "building":
      return "text-blue-500"
    case "deploying":
      return "text-orange-500"
    case "failed":
      return "text-red-500"
    case "active":
      return "text-green-500"
    default:
      return "text-neutral"
  }
}

function deploymentStatusBgColor(status: string) {
  switch (status) {
    case "pending":
      return "bg-yellow-100"
    case "building":
      return "bg-blue-100"
    case "deploying":
      return "bg-orange-100"
    case "failed":
      return "bg-red-100"
    case "active":
      return "bg-green-100"
    default:
      return "bg-neutral/10"
  }
}
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Deployments</div>
      <d-button @click="createDeployment">Create Deployment</d-button>
    </div>
    <div class="flex-1 overflow-auto">
      <div
        v-for="deployment in deployments"
        :key="deployment.id"
        class="hover:bg-surface-subtle border-neutral-subtle text-neutral grid grid-cols-[100px_100px_100px_3fr_1fr] items-center gap-2 border-b p-4 text-sm"
      >
        <div class="font-mono">{{ deployment.deploymentId }}</div>
        <div>
          <div
            :class="[
              deploymentStatusColor(deployment.status),
              deploymentStatusBgColor(deployment.status),
              'w-fit rounded px-1.5 py-0.5',
            ]"
          >
            {{ deployment.status }}
          </div>
        </div>
        <div class="font-mono">{{ deployment.githubCommit.slice(0, 7) }}</div>
        <div>
          <nuxt-link :to="formatDeploymentUrl(deployment)" external target="_blank">
            {{ deployment.domains?.[0]?.name }}
          </nuxt-link>
        </div>
        <div class="text-right">{{ renderDate(deployment.createdAt) }}</div>
      </div>
    </div>
  </div>
</template>
