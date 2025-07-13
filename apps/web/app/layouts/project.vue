<script setup lang="ts">
const route = useRoute()
const orgName = route.params.org
const projectName = route.params.project

const navigation = ref([
  { name: "Deployments", to: `/${orgName}/${projectName}` },
  { name: "Logs", to: `/${orgName}/${projectName}/logs` },
  { name: "Settings", to: `/${orgName}/${projectName}/settings` },
])
</script>

<template>
  <div class="bg-neutral-weak flex min-h-screen flex-1 flex-col">
    <DPageHeader :navigation="navigation" />
    <main class="flex flex-1 flex-col overflow-auto">
      <slot></slot>
    </main>
  </div>
</template>
