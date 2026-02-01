<script setup lang="ts">
const route = useRoute();

const orgSlug = computed<string>(() => route.params.org as string);
const projectSlug = computed<string>(() => route.params.project as string);

const links = computed(() => [
  {
    name: "Deployments",
    to: `/${orgSlug.value}/${projectSlug.value}`,
    active: route.path === `/${orgSlug.value}/${projectSlug.value}`,
  },
  {
    name: "Settings",
    to: `/${orgSlug.value}/${projectSlug.value}/settings`,
    active: route.path.startsWith(`/${orgSlug.value}/${projectSlug.value}/settings`),
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
        <div class="outline-neutral bg-surface flex-1 rounded-lg outline">
          <slot></slot>
        </div>
      </div>
    </div>
  </div>
</template>
