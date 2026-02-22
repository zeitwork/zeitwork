<script setup lang="ts">
import { NuxtLink } from "#components";

type Props = {
  to?: string;
  padding?: boolean;
};

const { to = undefined, padding = true } = defineProps<Props>();

const component = computed(() => {
  return to ? NuxtLink : "div";
});
</script>
<template>
  <component
    :is="component"
    class="hover:bg-surface-1 group border-edge bg-base flex min-h-14 border-b last:border-transparent"
    :class="padding ? 'px-4 py-3' : ''"
    :to="to"
  >
    <slot />
  </component>
</template>
