<script setup lang="ts">
import {
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectPortal,
  SelectContent,
  SelectViewport,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
} from "reka-ui";
import { ChevronDownIcon, ChevronUpIcon, CheckIcon } from "lucide-vue-next";

interface Props {
  options: { value: string | number | boolean | null; display: string }[];
  placeholder?: string;
  disabled?: boolean;
  size?: "sm" | "md";
}

const { options, placeholder, disabled, size = "md" } = defineProps<Props>();
const model = defineModel<string | null>();
const open = ref(false);
</script>

<template>
  <SelectRoot v-model="model" :open="open" @update:open="open = $event">
    <SelectTrigger
      :disabled="disabled"
      class="bg-base border-edge focus:bg-base focus:border-edge-strong flex cursor-default items-center justify-between rounded-lg border px-2.5 text-sm outline-none select-none"
      :class="[
        disabled
          ? 'bg-surface-2 cursor-not-allowed opacity-50'
          : 'hover:border-edge-strong',
        size === 'sm' ? 'h-7' : 'h-9',
      ]"
    >
      <SelectValue :placeholder="placeholder" />
      <div>
        <ChevronDownIcon v-if="!open" class="ml-2 size-4 text-secondary" />
        <ChevronUpIcon v-else class="ml-2 size-4 text-secondary" />
      </div>
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        position="popper"
        side="bottom"
        align="start"
        class="border-edge bg-base z-[9999] w-[var(--reka-select-trigger-width)] rounded-lg border shadow-sm"
        :side-offset="5"
        ref="selectContentRef"
      >
        <SelectViewport class="max-h-48 overflow-auto p-1">
          <SelectItem
            v-for="option in options"
            :key="String(option.value)"
            :value="option.value as string | number"
            class="hover:bg-surface-1 focus:bg-surface-1 text-primary flex cursor-default items-center justify-between rounded-md px-2.5 py-1.5 text-sm select-none focus:outline-0"
          >
            <SelectItemText>
              {{ option.display }}
            </SelectItemText>
            <SelectItemIndicator>
              <CheckIcon class="ml-2 size-4" />
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>
