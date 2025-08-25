<script setup lang="ts">
type Props = {
  wide?: boolean
}

const { wide = false } = defineProps<Props>()
</script>
<template>
  <div class="flex px-6">
    <div class="mx-auto w-full" :class="wide ? '' : 'max-w-7xl'">
      <slot></slot>
    </div>
  </div>
</template>
