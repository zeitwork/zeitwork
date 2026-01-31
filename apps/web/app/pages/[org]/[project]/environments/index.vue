<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
const orgId = route.params.org;
const projectSlug = route.params.project;

const { data: project } = await useFetch(`/api/projects/${projectSlug}`);
const { data: environments } = await useFetch(`/api/projects/${projectSlug}/environments`);
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Environments</div>
    </div>
    <div v-if="project" class="flex-1 overflow-auto p-4">
      <div>hello, world</div>
    </div>
  </div>
</template>
