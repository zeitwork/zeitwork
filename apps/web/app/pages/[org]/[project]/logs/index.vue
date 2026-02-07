<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const projectSlug = route.params.project as string;

// Fetch deployments to find the latest running one
const { data: deployments } = await useFetch(`/api/projects/${projectSlug}/deployments`);

const latestDeployment = computed(() => {
  if (!deployments.value?.length) return null;
  // Prefer running deployments, fall back to the most recent one
  const running = deployments.value.find((d) => d.status === "running");
  return running ?? deployments.value[0];
});

const { data: runtimeLogs, refresh: refreshRuntimeLogs } = await useFetch(
  () => (latestDeployment.value ? `/api/deployments/${latestDeployment.value.id}/logs` : null),
  { watch: [latestDeployment] },
);

useIntervalFn(() => {
  if (latestDeployment.value) {
    refreshRuntimeLogs();
  }
}, 1000);

const parsedRuntimeLogs = computed(
  () => runtimeLogs.value?.map((log) => parseAnsi(log.message)) ?? [],
);
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="flex items-center gap-4">
        <div class="text-neutral-strong flex items-center gap-1 text-sm">Runtime Logs</div>
        <span v-if="latestDeployment" class="text-xs text-neutral-500 font-mono">
          {{ latestDeployment.status }} &middot; {{ latestDeployment.id.slice(0, 8) }}
        </span>
      </div>
    </div>

    <div class="flex-1 overflow-auto bg-black p-4 font-mono text-sm">
      <div v-if="!latestDeployment" class="text-xs text-neutral-500">
        No deployments found.
      </div>
      <div v-else-if="runtimeLogs?.length === 0" class="text-xs text-neutral-500">
        No runtime logs available yet...
      </div>
      <pre
        v-for="(segments, index) in parsedRuntimeLogs"
        :key="index"
        class="text-xs text-neutral-400"
      ><span
          v-for="(segment, i) in segments"
          :key="i"
          :style="{
            color: segment.color ?? undefined,
            fontWeight: segment.bold ? 'bold' : undefined,
          }"
        >{{ segment.text }}</span></pre>
    </div>
  </div>
</template>
