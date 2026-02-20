<script setup lang="ts">
type Props = {
  name?: string;
  type?: "text" | "password" | "email" | "number" | "tel" | "date" | "datetime-local" | "url";
  required?: boolean;
  placeholder?: string;
  autocomplete?: string;
  label?: string;
  disabled?: boolean;
  hideArrows?: boolean;
  leading?: string;
  leadingBackground?: boolean;
  trailing?: string;
  trailingBackground?: boolean;
  min?: number;
  max?: number;
};

const {
  name,
  type = "text",
  required = false,
  placeholder = "",
  autocomplete = "",
  label = "",
  disabled = false,
  hideArrows,
  leading = "",
  trailing = "",
  leadingBackground = true,
  trailingBackground = true,
  min,
  max,
} = defineProps<Props>();

const [model, modifiers] = defineModel<string | number>({
  set(value) {
    if (modifiers.capitalize && typeof value === "string") {
      return value.charAt(0).toUpperCase() + value.slice(1);
    }
    if (modifiers.sanitize && typeof value === "string") {
      return value
        .toLowerCase()
        .replace(/ /g, "-")
        .replace(/[^a-z0-9-]/g, "");
    }
    if (modifiers.slug && typeof value === "string") {
      return value
        .toLowerCase()
        .replace(/ /g, "-")
        .replace(/[^a-z0-9-/]/g, "")
        .replace(/--+/g, "-");
    }

    return value;
  },
});

const inputElement = ref<HTMLInputElement | null>(null);

defineExpose({
  focus: () => {
    inputElement.value?.focus();
  },
});

const slots = useSlots();
const hasLeading = computed(() => {
  return !!leading || !!slots.leading;
});

const hasTrailing = computed(() => {
  return !!trailing || !!slots.trailing;
});
</script>

<template>
  <div
    class="bg-surface-1 border-edge text-primary text-copy has-[:focus]:bg-base has-[:focus]:border-edge-strong flex h-8 overflow-hidden rounded-md border leading-none transition-all outline-none"
    :class="[
      disabled
        ? 'bg-surface-2 cursor-not-allowed opacity-50'
        : 'hover:border-edge-strong',
    ]"
  >
    <div
      v-if="$slots.leading || leading"
      class="border-edge flex items-center px-2"
      :class="leadingBackground ? 'bg-surface-1' : ''"
    >
      <template v-if="leading">
        {{ leading }}
      </template>
      <template v-else>
        <slot name="leading"></slot>
      </template>
    </div>
    <input
      ref="inputElement"
      :id="name"
      :name="name"
      :type="type"
      :required="required"
      :placeholder="placeholder"
      :autocomplete="autocomplete"
      :label="label"
      :disabled="disabled"
      v-model="model"
      class="text-copy texte-neutral h-full w-full outline-none"
      :class="[
        hideArrows ? 'hide-arrows' : '',
        hasLeading ? 'pl-0' : 'pl-2.5',
        hasTrailing ? 'pr-0' : 'pr-2.5',
      ]"
    />

    <div
      v-if="$slots.trailing || trailing"
      class="border-edge flex items-center px-2"
      :class="trailingBackground ? 'bg-surface-1' : ''"
    >
      <template v-if="trailing">
        {{ trailing }}
      </template>
      <template v-else>
        <slot name="trailing"></slot>
      </template>
    </div>
  </div>
</template>
