<script setup lang="ts">
import { ExternalLinkIcon, GitCommitIcon, ClockIcon, CheckCircleIcon } from "lucide-vue-next"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectSlug = route.params.project as string

const { data: deployments } = await useFetch(`/api/deployments`, {
  query: {
    projectSlug,
  },
})
</script>

<template>
  <div class="flex-1 overflow-auto bg-black p-4 text-white">
    <pre>{{ deployments }}</pre>
  </div>
</template>
