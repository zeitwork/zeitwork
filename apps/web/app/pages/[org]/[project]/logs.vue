<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectId = route.params.project
</script>

<template>
  <div class="p-4">logs</div>
</template>
