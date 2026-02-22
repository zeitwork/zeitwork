<script lang="ts" setup>
type Props = {
  active?: boolean;
  background?: string;
  borderRadius?: string;
  hoverScale?: string;
  activeScale?: string;
  inactiveScale?: string;
  focus?: boolean;
  interactive?: boolean;
  innerClass?: string;
  disabled?: boolean;
  fullWidth?: boolean;
};

const {
  active = false,
  background = "bg-surface-1",
  borderRadius = "rounded-md",
  hoverScale = "group-hover/h:inset-0",
  activeScale = "inset-0",
  inactiveScale = "inset-1",
  focus = true,
  interactive = true,
  innerClass = "",
  disabled = false,
  fullWidth = false,
} = defineProps<Props>();
</script>

<template>
  <div
    class="text-primary text-copy group/h relative flex items-center"
    :class="[fullWidth ? 'w-full' : 'w-fit']"
  >
    <div
      class="absolute inset-0 transition-all"
      :class="[
        background,
        borderRadius,
        disabled ? '' : hoverScale,
        disabled ? '' : 'group-hover/h:opacity-100',
        interactive && !disabled ? 'group-active/h:inset-0.5' : '',
        active ? `opacity-100 ${activeScale}` : `opacity-0 ${inactiveScale}`,
        focus && !disabled
          ? 'group-has-focus-visible/h:inset-0 group-has-focus-visible/h:opacity-100 group-has-focus-visible/h:ring-focus/10 group-has-focus-visible/h:ring-2'
          : '',
        innerClass,
      ]"
    ></div>
    <div class="relative z-2 flex w-full items-center gap-2">
      <slot />
    </div>
  </div>
</template>
