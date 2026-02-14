<script setup lang="ts">
const route = useRoute();

const orgSlug = computed<string>(() => route.params.org as string);
const projectSlug = computed<string>(() => route.params.project as string);

const links = computed(() => [
  {
    name: "Deployments",
    to: `/${orgSlug.value}/${projectSlug.value}`,
    active:
      route.path === `/${orgSlug.value}/${projectSlug.value}` ||
      route.path.startsWith(`/${orgSlug.value}/${projectSlug.value}/deployments`),
  },
  {
    name: "Settings",
    to: `/${orgSlug.value}/${projectSlug.value}/settings`,
    active: route.path.startsWith(`/${orgSlug.value}/${projectSlug.value}/settings`),
  },
]);
</script>

<template>
  <div class="base-0 flex flex-col h-screen">
    <div class="flex flex-col h-full overflow-hidden p-1 pt-0">
      <d-navbar-header class="shrink-0" />
      <div
        class="base-1 flex min-h-0 flex-1 flex-col rounded-lg ring-1 ring-edge"
      >
        <d-navbar :links="links" />
        <div class="base-2 flex-1 rounded-lg ring-1 ring-edge overflow-auto">
          <slot></slot>
        </div>
      </div>
    </div>
  </div>
</template>
