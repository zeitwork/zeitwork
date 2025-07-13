<script setup lang="ts">
type Props = {
  name?: string
  type?: "text" | "password" | "email" | "number" | "tel" | "date" | "datetime-local" | "url"
  required?: boolean
  placeholder?: string
  autocomplete?: string
  label?: string
  disabled?: boolean
  hideArrows?: boolean
  leading?: string
  leadingBackground?: boolean
  trailing?: string
  trailingBackground?: boolean
  min?: number
  max?: number
}

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
  max
} = defineProps<Props>()

const [model, modifiers] = defineModel<string | number>({
  set(value) {
    if (modifiers.capitalize && typeof value === "string") {
      return value.charAt(0).toUpperCase() + value.slice(1)
    }
    if (modifiers.sanitize && typeof value === "string") {
      return value
        .toLowerCase()
        .replace(/ /g, "-")
        .replace(/[^a-z0-9-]/g, "")
    }
    if (modifiers.slug && typeof value === "string") {
      return value
        .toLowerCase()
        .replace(/ /g, "-")
        .replace(/[^a-z0-9-/]/g, "")
        .replace(/--+/g, "-")
    }

    return value
  }
})

const inputElement = ref<HTMLInputElement | null>(null)

defineExpose({
  focus: () => {
    inputElement.value?.focus()
  }
})
</script>

<template>
  <div
    class="bg-neutral border-neutral text-neutral text-copy has-[:focus]:bg-neutral flex h-9 overflow-hidden rounded-lg border leading-none transition-all outline-none has-[:focus]:border-blue-600 has-[:focus]:ring-2 has-[:focus]:ring-blue-300"
    :class="[
      disabled
        ? 'bg-neutral-strong cursor-not-allowed opacity-50'
        : 'hover:border-neutral-strong/30'
    ]"
  >
    <div
      v-if="$slots.leading || leading"
      class="border-neutral flex items-center border-r px-4"
      :class="leadingBackground ? 'bg-neutral-subtle' : ''"
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
      class="text-copy texte-neutral h-full w-full px-2.5 outline-none"
      :class="[hideArrows ? 'hide-arrows' : '']"
    />

    <div
      v-if="$slots.trailing || trailing"
      class="border-neutral flex items-center border-l px-3"
      :class="trailingBackground ? 'bg-neutral-subtle' : ''"
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
