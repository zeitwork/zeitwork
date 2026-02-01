<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";
import { DotIcon } from "lucide-vue-next";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const projectSlug = route.params.project as string;
const deploymentId = route.params.id as string;

const formattedId = computed(() => uuidToB58(deploymentId));

const { data: logs, refresh } = await useFetch(`/api/logs`, {
  query: {
    projectSlug,
    deploymentId,
  },
});

useIntervalFn(() => {
  refresh();
}, 1000);

const parsedLogs = computed(() =>
  logs.value?.map((log) => parseAnsi(log.message)) ?? []
);
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
      <pre v-if="logs?.length === 0" class="text-xs text-neutral-500">No build logs available yet</pre>
      <pre
        v-for="(segments, index) in parsedLogs"
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
