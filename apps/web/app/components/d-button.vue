<script setup lang="ts">
import { LoaderCircleIcon } from "lucide-vue-next"
import { NuxtLink } from "#components"
import { refDebounced } from "@vueuse/core"

const slots = useSlots()

interface Props {
  variant?: "primary" | "secondary" | "tertiary" | "danger" | "danger-light" | "transparent" | "outline"
  iconLeft?: Component
  to?: any
  size?: "XS" | "SM" | "MD" | "LG"
  type?: "submit" | "button"
  loading?: boolean
  disabled?: boolean
}

const props = defineProps<Props>()
const { variant = "primary", size = "LG", type = "button", loading = false } = props

const variantClasses: { [key: string]: string } = {
  primary:
    "bg-neutral-inverse text-neutral-inverse hover:bg-neutral-inverse-hover active:bg-neutral-inverse-hover border border-transparent",
  secondary: "bg-neutral-100 text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300 border border-transparent",
  tertiary: "bg-purple-100 text-purple-700 hover:bg-purple-200 border border-transparent",
  danger: "bg-red-700 text-white hover:bg-red-600 border border-transparent",
  "danger-light": "text-red-700 bg-red-100 hover:bg-red-200 active:bg-red-300 border border-transparent",
  transparent: "text-neutral-700 hover:bg-neutral-950/5 active:bg-neutral-200 border border-transparent",
  outline: "text-neutral-600 border border-neutral-100 hover:bg-neutral-50 active:bg-neutral-100",
}

const paddingClasses: { [key: string]: string } = {
  XS: "px-2",
  SM: "px-3",
  MD: "px-3",
  LG: "px-4",
}

const heightClasses: { [key: string]: string } = {
  XS: "h-6",
  SM: "h-7",
  MD: "h-8",
  LG: "h-9",
}

const widthClasses: { [key: string]: string } = {
  XS: "w-5",
  SM: "w-7",
  MD: "w-8",
  LG: "w-9",
}

const sizeClass = computed(() => {
  if (slots.default) {
    return [paddingClasses[size], heightClasses[size], "w-fit"]
  } else {
    return [heightClasses[size], widthClasses[size]]
  }
})

const isLoading = refDebounced(toRef(props, "loading"), 100)
</script>

<template>
  <component
    :is="to ? NuxtLink : 'button'"
    :type
    :to
    class="relative flex min-w-7 cursor-default items-center justify-center gap-2 rounded-md text-sm ring-blue-600 outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-2"
    :class="[sizeClass, variantClasses[variant], disabled ? 'pointer-events-none opacity-50' : '']"
    :disabled
  >
    <component v-if="iconLeft" :is="iconLeft" class="size-4" :class="{ 'opacity-0': isLoading }" />
    <slot name="leading"></slot>
    <div v-if="$slots.default" class="inline" :class="{ 'opacity-0': isLoading }">
      <slot></slot>
    </div>
    <slot name="trailing"></slot>
    <div v-if="isLoading" class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 transform">
      <LoaderCircleIcon class="size-5 animate-spin" />
    </div>
  </component>
</template>
