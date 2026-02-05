<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-edge-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="flex items-center gap-4">
        <div class="text-primary flex items-center gap-1 text-sm">Logs</div>
      </div>
    </div>

    <div class="flex-1 overflow-auto bg-surface-1 p-4 font-mono text-sm">
      <div class="text-xs text-tertiary">No logs available yet...</div>
    </div>
  </div>
</template>
