<script setup lang="ts">
const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);

const prefix = computed(() => `/${orgSlug.value}`);

const links = computed(() => [
  {
    name: "Projects",
    to: `${prefix.value}`,
    active: route.path === `${prefix.value}`,
  },
  {
    name: "Domains",
    to: `${prefix.value}/x/domains`,
    active: route.path === `${prefix.value}/x/domains`,
  },
  {
    name: "Storage",
    to: `${prefix.value}/x/storage`,
    active: route.path === `${prefix.value}/x/storage`,
  },
  {
    name: "Database",
    to: `${prefix.value}/x/databases`,
    active: route.path === `${prefix.value}/x/databases`,
  },
  {
    name: "Settings",
    to: `${prefix.value}/x/settings`,
    active: route.path === `${prefix.value}/x/settings`,
  },
]);
</script>

<template>
  <div class="bg-neutral-strong flex h-screen flex-col">
    <div class="flex flex-1 flex-col p-1 pt-0">
      <d-navbar-header class="shrink-0" />
      <div
        class="bg-neutral-subtle outline-neutral flex min-h-0 flex-1 flex-col rounded-lg outline"
      >
        <d-navbar :links="links" />
        <div class="outline-neutral bg-surface flex-1 overflow-auto rounded-lg outline">
          <slot></slot>
        </div>
      </div>
    </div>
  </div>
</template>
