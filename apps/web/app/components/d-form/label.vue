<script setup lang="ts">
type Props = {
  name?: string;
  required?: boolean;
};

const { name, required = false } = defineProps<Props>();
</script>
<template>
  <label
    :for="name"
    class="text-label text-primary flex justify-between gap-1 font-medium whitespace-nowrap"
  >
    <slot></slot>
    <span v-if="required" class="text-xs text-warn"> * </span>
  </label>
</template>
