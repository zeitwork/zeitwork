<script setup lang="ts">
const route = useRoute()
const orgSlug = computed<string>(() => route.params.org as string)

const links = [
  {
    name: "Projects",
    to: `/${orgSlug.value}`,
  },
  {
    name: "Domains",
    to: `/${orgSlug.value}/~/domains`,
  },
  {
    name: "Storage",
    to: `/${orgSlug.value}/~/storages`,
  },
  {
    name: "Database",
    to: `/${orgSlug.value}/~/databases`,
  },
  {
    name: "Settings",
    to: `/${orgSlug.value}/~/settings`,
  },
]
</script>

<template>
  <div class="flex h-screen flex-row bg-neutral-50">
    <d-sidebar :links="links" />
    <div class="x flex flex-1 flex-col">
      <div class="h-14 w-full"></div>
      <div class="mr-3 flex-1 overflow-auto rounded-t-lg border border-b-0 border-neutral-200 bg-white shadow-sm">
        <slot></slot>
      </div>
    </div>
  </div>
</template>

<style lang="postcss">
html,
body {
  overflow: hidden;
  @apply bg-neutral-50;
}
</style>
