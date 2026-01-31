<script setup lang="ts">
import { DotIcon } from "lucide-vue-next";
definePageMeta({
  layout: "project",
});

const route = useRoute();
const projectSlug = route.params.project as string;
const deploymentId = route.params.id as string;

const formattedId = computed(() => uuidToB58(deploymentId));

const { data: logs } = await useFetch(`/api/logs`, {
  query: {
    projectSlug,
    deploymentId,
  },
});
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
      <pre class="text-xs text-neutral-500">{{ logs }}</pre>
    </div>
  </div>
</template>
