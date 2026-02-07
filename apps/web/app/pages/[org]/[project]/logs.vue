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
    <div class="border-edge-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-primary text-sm">Logs</div>
      <!-- <d-button @click="createDeployment">Create Deployment</d-button> -->
    </div>
  </div>
</template>
