<script setup lang="ts">
import { useTimeoutFn, useScroll } from "@vueuse/core"
import { ref, computed, watch, nextTick } from "vue"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const deploymentId = route.params.id as string

// Fetch deployment details
const { data: deployment } = await useFetch(`/api/deployments/${deploymentId}`)

// Fetch logs with context filter
const logContext = ref<"all" | "build" | "runtime">("all")
const { data: logs, refresh: refreshLogs } = await useFetch(`/api/deployments/${deploymentId}/logs`, {
  query: {
    context: logContext,
  },
})

// Auto-scroll setup
const logContainer = ref<HTMLElement>()
const { arrivedState } = useScroll(logContainer, { behavior: "smooth" })
const autoScroll = ref(true)

// Scroll to bottom when new logs arrive
watch(logs, async () => {
  if (autoScroll.value && logContainer.value) {
    await nextTick()
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
})

// Disable auto-scroll if user scrolls up
watch(() => arrivedState.bottom, (isAtBottom) => {
  if (!isAtBottom) {
    autoScroll.value = false
  }
})

// Re-enable auto-scroll when user scrolls to bottom
const scrollToBottom = () => {
  autoScroll.value = true
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

// Polling for logs
const { isPending, start, stop } = useTimeoutFn(() => {
  refreshLogs()
}, 2000)

start()

watch(isPending, (newVal) => {
  if (newVal) {
    start()
  } else {
    start()
  }
})

function renderDate(date: string) {
  return Intl.DateTimeFormat("en-DE", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(date))
}

function deploymentStatusColor(status: string) {
  switch (status) {
    case "pending":
      return "text-yellow-600"
    case "building":
      return "text-blue-600"
    case "deploying":
      return "text-orange-600"
    case "failed":
      return "text-red-600"
    case "active":
      return "text-green-600"
    case "inactive":
      return "text-neutral-500"
    default:
      return "text-neutral-700"
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
    case "inactive":
      return "bg-neutral-100"
    default:
      return "bg-neutral-100"
  }
}

const formattedLogs = computed(() => {
  if (!logs.value) return []
  return logs.value.map((log: any) => ({
    ...log,
    timestamp: new Date(log.loggedAt).toLocaleTimeString("en-DE", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    }),
  }))
})
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <!-- Header -->
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="flex items-center gap-4">
        <nuxt-link
          :to="`/${route.params.org}/${route.params.project}/deployments`"
          class="text-neutral-strong hover:text-neutral text-sm"
        >
          ← Back to Deployments
        </nuxt-link>
        <div class="text-neutral-strong text-sm font-semibold">Deployment {{ deployment?.deploymentId }}</div>
      </div>
    </div>

    <!-- Deployment Info -->
    <div v-if="deployment" class="border-neutral-subtle border-b bg-white p-4">
      <div class="flex items-center gap-4">
        <div
          :class="[
            deploymentStatusColor(deployment.status),
            deploymentStatusBgColor(deployment.status),
            'w-fit rounded px-2 py-1 text-sm font-medium',
          ]"
        >
          {{ deployment.status }}
        </div>
        <div class="text-neutral text-sm">Commit: <span class="font-mono">{{ deployment.githubCommit.slice(0, 7) }}</span></div>
        <div class="text-neutral text-sm">Created: {{ renderDate(deployment.createdAt) }}</div>
        <div v-if="deployment.domains?.[0]" class="text-neutral text-sm">
          Domain: <a :href="`http://${deployment.domains[0].name}`" target="_blank" class="text-blue-600 hover:underline">{{ deployment.domains[0].name }}</a>
        </div>
      </div>
    </div>

    <!-- Log Context Filter -->
    <div class="border-neutral-subtle flex items-center gap-2 border-b bg-white p-2">
      <button
        @click="logContext = 'all'"
        :class="[
          'rounded px-3 py-1 text-sm',
          logContext === 'all'
            ? 'bg-blue-600 text-white'
            : 'bg-neutral-100 text-neutral-700 hover:bg-neutral-200',
        ]"
      >
        All Logs
      </button>
      <button
        @click="logContext = 'build'"
        :class="[
          'rounded px-3 py-1 text-sm',
          logContext === 'build'
            ? 'bg-blue-600 text-white'
            : 'bg-neutral-100 text-neutral-700 hover:bg-neutral-200',
        ]"
      >
        Build Logs
      </button>
      <button
        @click="logContext = 'runtime'"
        :class="[
          'rounded px-3 py-1 text-sm',
          logContext === 'runtime'
            ? 'bg-blue-600 text-white'
            : 'bg-neutral-100 text-neutral-700 hover:bg-neutral-200',
        ]"
      >
        Runtime Logs
      </button>
      <div class="ml-auto flex items-center gap-2">
        <button
          v-if="!autoScroll"
          @click="scrollToBottom"
          class="rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700"
        >
          ↓ Scroll to Bottom
        </button>
        <span class="text-neutral text-xs">Auto-refresh: {{ isPending ? 'active' : 'paused' }}</span>
      </div>
    </div>

    <!-- Logs Terminal -->
    <div
      ref="logContainer"
      class="flex-1 overflow-auto bg-black p-4 font-mono text-sm text-green-400"
    >
      <div v-if="!formattedLogs || formattedLogs.length === 0" class="text-neutral-500">
        No logs available yet...
      </div>
      <div v-for="log in formattedLogs" :key="log.id" class="whitespace-pre-wrap">
        <span class="text-neutral-500">[{{ log.timestamp }}]</span>
        <span v-if="log.level" :class="log.level === 'error' ? 'text-red-400' : 'text-blue-400'" class="ml-2">[{{ log.level }}]</span>
        <span class="ml-2">{{ log.message }}</span>
      </div>
    </div>
  </div>
</template>

