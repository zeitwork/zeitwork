<script setup lang="ts">
const route = useRoute()
const orgId = computed<string>(() => route.params.org as string)

const projects = computed(() => [])
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <div class="flex gap-2">
        <DInput class="flex-1" placeholder="Search Projects..." />
        <DButton :to="`/${orgId}/new`">Add Project</DButton>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <!-- <pre>{{ data }}</pre> -->
        <DProjectCard v-for="project in projects" :key="project.id" :project="project" />
      </div>
    </div>
  </DPageWrapper>
</template>
