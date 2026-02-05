<script setup lang="ts">
import {
  ComboboxAnchor,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxPortal,
  ComboboxRoot,
  ComboboxTrigger,
  ComboboxViewport,
} from "reka-ui";

import { ChevronUpDownIcon } from "@heroicons/vue/16/solid";

import { ref, computed, watch } from "vue";

interface Props {
  modelValue?: string;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  items?: Array<{ value: string; label: string }>;
  filterFunction?: (items: any[], term: string) => any[];
  displayValue?: (value: string) => string;
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: "Select an option",
  searchPlaceholder: "Search...",
  emptyText: "No results found",
});

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const open = ref(false);
const isClosing = ref(false);
const searchTerm = ref("");
const frozenSearchTerm = ref("");

const modelValue = computed({
  get: () => props.modelValue,
  set: (value) => emit("update:modelValue", value as string),
});

const selectedLabel = computed(() => {
  if (!props.modelValue || !props.items) return props.placeholder;
  const item = props.items.find((i) => i.value === props.modelValue);
  return item ? item.label : props.placeholder;
});

const selectedItem = computed(() => {
  if (!props.modelValue || !props.items) return null;
  return props.items.find((i) => i.value === props.modelValue) || null;
});

const filteredItems = computed(() => {
  if (!props.items) return [];

  const term = isClosing.value ? frozenSearchTerm.value : searchTerm.value;
  if (!term) return props.items;

  if (props.filterFunction) {
    return props.filterFunction(props.items, term);
  }

  return props.items.filter((item) => item.label.toLowerCase().includes(term.toLowerCase()));
});

const customFilterFunction = (items: any[], term: string) => {
  return items;
};

const handleModelValueUpdate = (value: string) => {
  frozenSearchTerm.value = searchTerm.value;
  emit("update:modelValue", value);
};

watch(open, (newOpen, oldOpen) => {
  if (oldOpen && !newOpen) {
    isClosing.value = true;
    setTimeout(() => {
      isClosing.value = false;
      searchTerm.value = "";
    }, 300);
  }
});
</script>

<template>
  <ComboboxRoot
    :model-value="props.modelValue"
    @update:model-value="handleModelValueUpdate"
    v-model:open="open"
    v-model:search-term="searchTerm"
    :filter-function="customFilterFunction"
    :ignore-filter="true"
    :display-value="displayValue"
    :reset-search-term-on-blur="false"
    :reset-search-term-on-select="false"
  >
    <ComboboxAnchor as-child>
      <ComboboxTrigger
        class="bg-surface-weak border-neutral text-neutral text-copy focus:bg-neutral hover:border-neutral-strong/15 disabled:bg-neutral-strong flex h-8 w-full min-w-40 items-center justify-between gap-1 rounded-md border px-2.5 py-2 leading-none transition-all outline-none focus:border-neutral-300 focus:ring-2 focus:ring-neutral-200 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <span class="flex">
          <slot
            name="trigger"
            :selected-item="selectedItem"
            :selected-label="selectedLabel"
            :placeholder="props.placeholder"
          >
            {{ selectedLabel }}
          </slot>
        </span>
        <ChevronUpDownIcon class="size-4" />
      </ComboboxTrigger>
    </ComboboxAnchor>

    <ComboboxPortal>
      <ComboboxContent
        :side-offset="4"
        class="bg-surface border-neutral text-neutral-strong data-[state=open]:animate-fade-in-from-top data-[state=closed]:animate-fade-out relative z-[200] max-h-96 w-[var(--reka-combobox-trigger-width)] overflow-hidden rounded-md border shadow-md"
        position="popper"
      >
        <ComboboxViewport class="p-0.5">
          <ComboboxInput
            v-model="searchTerm"
            :display-value="() => ''"
            class="text-copy text-neutral placeholder:text-neutral-weak flex h-8 w-full rounded-xl bg-transparent px-2 leading-none outline-none disabled:cursor-not-allowed disabled:opacity-50"
            :placeholder="searchPlaceholder"
          />
          <ComboboxEmpty class="text-copy text-neutral-weak py-6 text-center">
            {{ emptyText }}
          </ComboboxEmpty>
          <ComboboxGroup>
            <slot
              v-if="$slots.default"
              :is-closing="isClosing"
              :search-term="searchTerm"
              :filtered-items="filteredItems"
              :items="filteredItems"
            />
            <template v-else>
              <DComboboxItem
                v-for="item in filteredItems"
                :key="item.value"
                :value="item.value"
                :label="item.label"
              />
            </template>
          </ComboboxGroup>
          <slot
            name="footer"
            :is-closing="isClosing"
            :search-term="searchTerm"
            :filtered-items="filteredItems"
          />
        </ComboboxViewport>
      </ComboboxContent>
    </ComboboxPortal>
  </ComboboxRoot>
</template>
