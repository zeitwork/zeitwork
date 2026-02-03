<script setup lang="ts">
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
    <DHeader title="Deployment details">
      <template #leading>
        <span class="text-copy text-neutral-subtle">{{ formattedId }}</span>
      </template>
    </DHeader>

    <!-- Logs Terminal -->
    <div class="flex-1 overflow-auto bg-black p-4 font-mono text-sm">
      <div class="text-xs text-neutral-500">No logs available yet...</div>
      <pre class="text-xs text-neutral-500">{{ logs }}</pre>
    </div>
  </div>
</template>
