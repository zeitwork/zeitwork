<script setup lang="ts">
import { DotIcon } from "lucide-vue-next"
definePageMeta({
  layout: "environment",
})

const route = useRoute()
const deploymentId = route.params.id as string

const formattedId = computed(() => uuidToB58(deploymentId))

const prefix = computed(() => `/${route.params.org}/${route.params.project}/${route.params.env}`)
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <!-- Header -->
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-1 text-sm">
          <span class="text-neutral-strong">Deployment details</span>
          <DotIcon class="text-neutral-subtle size-4" />
          <span class="text-neutral-subtle"> {{ formattedId }}</span>
        </div>
      </div>
    </div>

    <!-- Logs Terminal -->
    <div class="flex-1 overflow-auto bg-black p-4 font-mono text-sm">
      <div class="text-xs text-neutral-500">No logs available yet...</div>
    </div>
  </div>
</template>
