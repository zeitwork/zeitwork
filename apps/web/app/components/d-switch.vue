<template>
  <button
    type="button"
    :class="[
      'group focus-visble:ring-2 relative box-content inline-flex w-10 flex-shrink-0 cursor-pointer rounded-full border p-0.5 transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-blue-600 focus-visible:ring-offset-2',
      modelValue ? 'bg-neutral-inverse border-neutral' : 'bg-neutral-subtle border-neutral'
    ]"
    :aria-checked="modelValue"
    @click="toggle"
  >
    <span
      :class="[
        'ring-0Atransition pointer-events-none h-5 w-5 transform rounded-full bg-white shadow duration-200 ease-in-out group-hover:shadow-md',
        modelValue ? 'translate-x-5' : 'translate-x-0'
      ]"
    />
  </button>
</template>

<script setup>
const props = defineProps({
  modelValue: Boolean
})

const emit = defineEmits(["update:modelValue"])

const toggle = () => {
  emit("update:modelValue", !props.modelValue)
}
</script>
