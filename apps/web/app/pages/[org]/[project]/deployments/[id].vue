<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";
import { DotIcon, HammerIcon, PlayIcon } from "lucide-vue-next";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const projectSlug = route.params.project as string;
const deploymentId = route.params.id as string;

const formattedId = computed(() => uuidToB58(deploymentId));

const { data: buildLogs, refresh: refreshBuildLogs } = await useFetch(`/api/logs`, {
  query: {
    projectSlug,
    deploymentId,
  },
});

useIntervalFn(() => {
  refreshBuildLogs();
}, 1000);

const parsedBuildLogs = computed(() => buildLogs.value?.map((log) => parseAnsi(log.message)) ?? []);
</script>

<template>
  <div class="flex h-full flex-col max-h-screen">
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

    <div class="flex-1 overflow-auto">
      <!-- Build Logs Section -->
      <div class="border-neutral-subtle border-b">
        <div
          class="border-neutral-subtle flex items-center gap-2 border-b bg-neutral-950 px-4 py-2"
        >
          <HammerIcon class="size-4 text-neutral-400" />
          <span class="text-sm font-medium text-neutral-300">Build Logs</span>
        </div>
        <div class="bg-black p-4 font-mono text-sm">
          <pre v-if="buildLogs?.length === 0" class="text-xs text-neutral-500">
No build logs available yet...</pre
          >
          <pre
            v-for="(segments, index) in parsedBuildLogs"
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

      <!-- Runtime Logs Section -->
      <div>
        <div
          class="border-neutral-subtle flex items-center gap-2 border-b bg-neutral-950 px-4 py-2"
        >
          <PlayIcon class="size-4 text-neutral-400" />
          <span class="text-sm font-medium text-neutral-300">Runtime Logs</span>
        </div>
        <div class="bg-black p-4 font-mono text-sm">
          <pre class="text-xs text-neutral-500">Runtime logs not available yet. Coming soon...</pre>
        </div>
      </div>
    </div>
  </div>
</template>
