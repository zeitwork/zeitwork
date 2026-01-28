<script setup lang="ts">
const route = useRoute()

const orgSlug = computed<string>(() => route.params.org as string)
const projectSlug = computed<string>(() => route.params.project as string)
const environmentName = computed<string>(() => route.params.env as string)

const prefix = computed(() => `/${orgSlug.value}/${projectSlug.value}/${environmentName.value}`)

const links = computed(() => [
  {
    name: "Environment",
    to: `${prefix.value}`,
    active: route.path === `${prefix.value}`,
  },
  {
    name: "Deployments",
    to: `${prefix.value}/deployments`,
    active: route.path.startsWith(`${prefix.value}/deployments`),
  },
  {
    name: "Logs",
    to: `${prefix.value}/logs`,
    active: route.path === `${prefix.value}/logs`,
  },
  {
    name: "Settings",
    to: `${prefix.value}/settings`,
    active: route.path === `${prefix.value}/settings`,
  },
])
</script>

<template>
  <div class="bg-neutral-strong flex h-screen flex-col">
    <div class="flex flex-1 flex-col p-1 pt-0">
      <d-navbar-header class="shrink-0" />
      <div class="bg-neutral-subtle outline-neutral flex min-h-0 flex-1 flex-col rounded-lg outline">
        <d-navbar :links="links" />
        <div class="outline-neutral bg-surface flex-1 overflow-auto rounded-lg outline">
          <slot></slot>
        </div>
      </div>
    </div>
  </div>
</template>
