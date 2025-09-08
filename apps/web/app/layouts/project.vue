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
  <div class="bg-neutral-strong flex h-screen flex-col">
    <d-navbar :links="links" />
    <div class="flex flex-1 flex-col overflow-auto p-2 pt-0">
      <div class="border-neutral h-full flex-1 overflow-auto rounded-lg border bg-white">
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
