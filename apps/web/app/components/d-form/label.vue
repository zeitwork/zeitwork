<script setup lang="ts">
type Props = {
  name?: string
  required?: boolean
}

const { name, required = false } = defineProps<Props>()
</script>
<template>
  <label
    :for="name"
    class="text-label text-neutral flex justify-between gap-1 font-medium whitespace-nowrap"
  >
    <slot></slot>
    <span
      v-if="required"
      class="text-xs text-orange-500"
    >
      *
    </span>
  </label>
</template>
