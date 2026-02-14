<script setup lang="ts">
type Props = {
  rows?: number;
  cols?: number;
  placeholder?: string;
  disabled?: boolean;
};

const { rows = 5, cols = 50, placeholder = "", disabled = false } = defineProps<Props>();

const model = defineModel<string>();
</script>
<template>
  <textarea
    v-model="model"
    class="text-primary bg-base text-copy border-edge focus:bg-base focus:border-edge-strong rounded-lg border p-2 transition-colors outline-none"
    :rows="rows"
    :cols="cols"
    :placeholder="placeholder"
    :disabled="disabled"
  ></textarea>
</template>
