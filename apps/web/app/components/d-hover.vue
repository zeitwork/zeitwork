<script lang="ts" setup>
type Props = {
  active?: boolean
  background?: string
  borderRadius?: string
  hoverScale?: string
  activeScale?: string
  inactiveScale?: string
  ring?: boolean
}

const {
  active = false,
  background = "bg-neutral-subtle",
  borderRadius = "rounded-[10px]",
  hoverScale = "group-hover:inset-0",
  activeScale = "inset-0",
  inactiveScale = "inset-1",
  ring = true,
} = defineProps<Props>()
</script>

<template>
  <div class="text-neutral text-copy group relative flex items-center">
    <div
      class="absolute inset-0 transition-all group-hover:opacity-100"
      :class="[
        background,
        borderRadius,
        hoverScale,
        active ? `opacity-100 ${activeScale}` : `opacity-0 ${inactiveScale}`,
        ring ? 'group-active:inset-[-2px]' : '',
      ]"
    ></div>
    <div class="relative z-2 flex w-full items-center gap-2">
      <slot />
    </div>
  </div>
</template>
