<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
const orgId = route.params.org;
const projectId = route.params.project;
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Logs</div>
      <!-- <d-button @click="createDeployment">Create Deployment</d-button> -->
    </div>
  </div>
</template>
