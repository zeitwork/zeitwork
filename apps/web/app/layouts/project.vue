<script setup lang="ts">
import { Cog8ToothIcon, CubeTransparentIcon } from "@heroicons/vue/16/solid";

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
    icon: CubeTransparentIcon,
  },
  {
    name: "Settings",
    to: `/${orgSlug.value}/${projectSlug.value}/settings`,
    active: route.path.startsWith(`/${orgSlug.value}/${projectSlug.value}/settings`),
    icon: Cog8ToothIcon,
  },
]);
</script>

<template>
  <div class="bg-neutral-strong flex h-screen flex-col">
    <div class="flex flex-1 flex-col p-1 pt-0">
      <DNavbarHeader class="shrink-0" />
      <div
        class="bg-neutral-subtle outline-neutral flex min-h-0 flex-1 flex-col rounded-lg outline"
      >
        <DNavbar :links="links" />
        <div class="outline-neutral bg-surface flex-1 overflow-auto rounded-lg outline">
          <slot></slot>
        </div>
      </div>
    </div>
  </div>
</template>
