<script setup lang="ts">
import { NuxtLink } from "#components"

type Props = {
  to?: string
  type?: "button" | "submit" | "reset"
  variant?: "primary" | "secondary"
}

const { to, variant = "secondary", type = "button" } = defineProps<Props>()

const variantClasses: { [key: string]: string } = {
  primary: "bg-white text-neutral-900 hover:bg-neutral-100 active:bg-neutral-200 border border-transparent",
  secondary: "bg-neutral-700 text-neutral-100 hover:bg-neutral-600 active:bg-neutral-500 border border-transparent",
}
</script>

<template>
  <component
    :is="to ? NuxtLink : 'button'"
    :to="to"
    class="inline-flex h-9 cursor-pointer items-center rounded-xl px-3 text-sm transition-all duration-100 select-none"
    :class="variantClasses[variant]"
    :type="type"
  >
    <div>
      <slot></slot>
    </div>
  </component>
</template>
