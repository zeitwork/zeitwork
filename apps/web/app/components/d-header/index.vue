<script setup lang="ts">
import type { Component } from "vue";

interface Props {
  icon?: Component;
  title?: string;
}

const { icon, title } = defineProps<Props>();
const slots = useSlots();
</script>

<template>
  <header class="border-neutral flex items-center border-b p-2 h-12.5">
    <div class="flex flex-1 items-center gap-2 px-1.5">
      <div class="flex items-center gap-2">
        <component v-if="icon" :is="icon" class="text-neutral size-4" />
        <span v-if="title" class="text-copy text-neutral">{{ title }}</span>
      </div>
      <slot v-if="slots.leading" name="leading" />
    </div>
    <div v-if="slots.trailing" class="flex items-center gap-2">
      <slot name="trailing" />
    </div>
  </header>
</template>
