<script setup lang="ts">
import { ExternalLinkIcon, GitCommitIcon, ClockIcon, CheckCircleIcon } from "lucide-vue-next"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectId = route.params.project as string

const { data: deployments } = await useFetch(`/api/deployments`, {
  query: {
    projectId,
  },
})
</script>

<template>
  <div class="p-4">
    <pre>{{ deployments }}</pre>
  </div>
</template>
