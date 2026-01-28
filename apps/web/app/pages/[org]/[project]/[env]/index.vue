<script setup lang="ts">
definePageMeta({
  layout: "environment",
})

const route = useRoute()
const orgId = route.params.org
const projectSlug = route.params.project
const environmentName = route.params.env

const { data: environment } = await useFetch(`/api/projects/${projectSlug}/environments/${environmentName}`)
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Environment</div>
    </div>
    <div v-if="environment" class="flex-1 overflow-auto p-4">
      <pre class="rounded bg-black p-4 font-mono text-xs text-white">{{ environment }}</pre>
    </div>
  </div>
</template>
