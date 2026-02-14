<script setup lang="ts">
import { LoaderCircleIcon } from "lucide-vue-next";
import { NuxtLink } from "#components";
import { refDebounced } from "@vueuse/core";

const slots = useSlots();

interface Props {
  variant?:
    | "primary"
    | "secondary"
    | "tertiary"
    | "danger"
    | "danger-light"
    | "transparent"
    | "outline";
  iconLeft?: Component;
  to?: any;
  size?: "xs" | "sm" | "md";
  type?: "submit" | "button";
  loading?: boolean;
  disabled?: boolean;
}

const props = defineProps<Props>();
const {
  variant = "primary",
  size = "md",
  type = "button",
  loading = false,
} = props;

const variantClasses: { [key: string]: string } = {
  primary:
    "bg-surface-1 text-primary hover:bg-surface-2 border border-edge shadow-xs active:bg-surface-3",
  secondary:
    "bg-surface-1 text-primary hover:bg-surface-2 active:bg-base border border-transparent",
  transparent:
    "text-secondary hover:bg-surface-1 active:bg-inverse/10 border !border-transparent",
  danger:
    "bg-danger text-danger-on hover:bg-danger-strong border border-transparent active:bg-danger-strong",
  "danger-light":
    "text-danger bg-danger-subtle hover:bg-danger/30 active:bg-danger/50 border border-transparent",
  outline:
    "text-secondary border border-edge-subtle hover:bg-inverse/5 active:bg-inverse/10",
};

const paddingClasses: { [key: string]: string } = {
  xs: "px-2",
  sm: "px-3",
  md: "px-3",
};

const heightClasses: { [key: string]: string } = {
  xs: "h-6",
  sm: "h-7",
  md: "h-8",
};

const widthClasses: { [key: string]: string } = {
  xs: "w-5",
  sm: "w-7",
  md: "w-8",
};

const sizeClass = computed(() => {
  if (slots.default) {
    return [paddingClasses[size], heightClasses[size], "w-fit"];
  } else {
    return [heightClasses[size], widthClasses[size]];
  }
});

const isLoading = refDebounced(toRef(props, "loading"), 100);
</script>

<template>
  <component
    :is="to ? NuxtLink : 'button'"
    :type
    :to
    class="relative flex min-w-fit cursor-default items-center justify-center gap-2 rounded-md text-sm whitespace-pre ring-focus outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-2"
    :class="[
      sizeClass,
      variantClasses[variant],
      disabled ? 'pointer-events-none opacity-50' : '',
    ]"
    :disabled
  >
    <component
      v-if="iconLeft"
      :is="iconLeft"
      class="size-4"
      :class="{ 'opacity-0': isLoading }"
    />
    <slot name="leading"></slot>
    <div
      v-if="$slots.default"
      class="inline"
      :class="{ 'opacity-0': isLoading }"
    >
      <slot></slot>
    </div>
    <slot name="trailing"></slot>
    <div
      v-if="isLoading"
      class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 transform"
    >
      <LoaderCircleIcon class="size-5 animate-spin" />
    </div>
  </component>
</template>
