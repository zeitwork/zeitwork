<script setup lang="ts">
import type { Component } from "vue";

type Props = {
  title: string;
  description?: string;
  actionLabel?: string;
  actionIcon?: Component;
};

type Emits = {
  action: [];
};

const { title, description, actionLabel, actionIcon } = defineProps<Props>();
const emit = defineEmits<Emits>();
</script>

<template>
  <div class="flex flex-col items-center justify-center py-20">
    <h3 class="text-title-sm text-primary">{{ title }}</h3>
    <p v-if="description" class="text-copy text-secondary mt-1">{{ description }}</p>
    <DButton v-if="actionLabel" class="mt-4" :icon-left="actionIcon" @click="emit('action')">
      {{ actionLabel }}
    </DButton>
  </div>
</template>
