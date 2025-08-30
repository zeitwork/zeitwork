<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectSlug = route.params.project

const { data: project, refresh } = await useFetch(`/api/projects/${projectSlug}`)
</script>

<template>
  <div class="p-4">
    <pre>{{ project }}</pre>
  </div>
</template>
