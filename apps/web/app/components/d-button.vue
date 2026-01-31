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
const { variant = "primary", size = "md", type = "button", loading = false } = props;

const variantClasses: { [key: string]: string } = {
  primary:
    "bg-surface text-neutral hover:bg-surface-subtle border border-neutral shadow-xs active:bg-surface-strong",
  secondary:
    "bg-neutral-subtle text-neutral hover:bg-neutral-strong active:bg-neutral border border-transparent",
  transparent:
    "text-neutral-subtle hover:bg-neutral-subtle active:bg-neutral-strong border !border-transparent",
  danger: "bg-danger text-white hover:bg-danger border border-transparent active:bg-danger-strong",
  "danger-light":
    "text-danger bg-red-100 hover:bg-danger/30 active:bg-danger/50 border border-transparent",
  outline:
    "text-neutral-subtle border border-neutral-subtle hover:bg-neutral-subtle active:bg-neutral-strong",
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
    class="relative flex min-w-fit cursor-default items-center justify-center gap-2 rounded-md text-sm whitespace-pre ring-blue-600 outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-2"
    :class="[sizeClass, variantClasses[variant], disabled ? 'pointer-events-none opacity-50' : '']"
    :disabled
  >
    <component v-if="iconLeft" :is="iconLeft" class="size-4" :class="{ 'opacity-0': isLoading }" />
    <slot name="leading"></slot>
    <div v-if="$slots.default" class="inline" :class="{ 'opacity-0': isLoading }">
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
