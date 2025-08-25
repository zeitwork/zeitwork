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
  <div class="bg-neutral-strong flex h-screen flex-row">
    <d-sidebar :links="links" />
    <div class="flex flex-1 flex-col py-2 pr-2 pl-0">
      <div class="border-neutral flex-1 overflow-auto rounded-lg border bg-white">
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
