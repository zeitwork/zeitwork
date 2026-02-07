<script setup lang="ts">
import type { Component } from "vue";

type Props = {
  icon?: Component;
  title?: string;
  description?: string;
};

const { icon, title, description } = defineProps<Props>();
const slots = useSlots();
</script>

<template>
  <header class="border-edge flex h-12.5 items-center border-b p-2">
    <div class="flex flex-1 items-center gap-2 px-1.5">
      <div class="flex items-center gap-2">
        <component v-if="icon" :is="icon" class="text-primary size-4" />
        <span v-if="title" class="text-copy text-primary">{{ title }}</span>
      </div>
      <span
        v-if="description"
        class="line-clamp-1 text-copy text-secondary"
        >{{ description }}</span
      >
      <slot v-if="slots.leading" name="leading" />
    </div>
    <div v-if="slots.trailing" class="flex items-center gap-2">
      <slot name="trailing" />
    </div>
  </header>
</template>
