<script setup lang="ts">
const route = useRoute()
const orgName = route.params.org

const navigation = ref([
  { name: "Projects", to: `/${orgName}` },
  { name: "Domains", to: `/${orgName}/~/domains` },
  { name: "Storage", to: `/${orgName}/~/storage` },
  { name: "Databases", to: `/${orgName}/~/databases` },
  { name: "Settings", to: `/${orgName}/~/settings` },
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
