<script setup lang="ts">
type Props = {
  name: string;
  to: string;
};
const { to, name } = defineProps<Props>();
</script>
<template>
  <NuxtLink
    :to="to"
    class="text-secondary hover:text-primary hover:bg-surface-1 text-copy line-clamp-1 max-w-50 rounded-lg px-2 py-1"
    :title="name"
  >
    {{ name }}
  </NuxtLink>
</template>
