<script setup lang="ts">
const route = useRoute()
const orgSlug = computed<string>(() => route.params.org as string)
const projectSlug = computed<string>(() => route.params.project as string)

const links = [
  {
    name: "Overview",
    to: `/${orgSlug.value}/${projectSlug.value}`,
  },
  {
    name: "Deployments",
    to: `/${orgSlug.value}/${projectSlug.value}/deployments`,
  },
  {
    name: "Logs",
    to: `/${orgSlug.value}/${projectSlug.value}/logs`,
  },
  {
    name: "Settings",
    to: `/${orgSlug.value}/${projectSlug.value}/settings`,
  },
]
</script>

<template>
  <div class="flex h-screen flex-row bg-neutral-50">
    <d-sidebar :links="links" />
    <div class="x flex flex-1 flex-col">
      <div class="flex h-14 w-full items-center px-4">
        <div class="text-sm text-neutral-500">{{ projectSlug }}</div>
      </div>
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
