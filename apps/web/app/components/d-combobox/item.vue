<script setup lang="ts">
import { ComboboxItem, ComboboxItemIndicator } from "reka-ui"

import { CheckIcon } from "@heroicons/vue/16/solid"

interface Props {
  value: string
  label?: string
  disabled?: boolean
}

defineProps<Props>()
</script>

<template>
  <ComboboxItem
    :value="value"
    :disabled="disabled"
    class="text-copy text-neutral group hover:bg-neutral-subtle data-[highlighted]:bg-neutral-subtle relative flex w-full cursor-pointer items-center rounded-sm px-2 py-1.5 leading-none transition-colors outline-none select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
  >
    <div
      class="bg-neutral-strong absolute inset-2 rounded opacity-0 transition-all group-data-[highlighted]:inset-0 group-data-[highlighted]:opacity-100"
    ></div>
    <div class="z-10 flex w-full items-center">
      <div class="flex flex-1 items-center gap-2 overflow-hidden whitespace-nowrap">
        <slot name="leading"></slot>
        <div class="truncate py-0.5">
          <slot>{{ label }}</slot>
        </div>
      </div>
      <div class="grid size-5 min-w-5 place-items-center justify-center">
        <ComboboxItemIndicator>
          <CheckIcon class="text-neutral-weak size-4" />
        </ComboboxItemIndicator>
      </div>
    </div>
  </ComboboxItem>
</template>
