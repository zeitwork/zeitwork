<script setup lang="ts">
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
  DialogTrigger,
} from "reka-ui";

type Size = "default" | "lg";

type Props = {
  size?: Size;
};

const { size = "default" } = defineProps<Props>();
const open = defineModel<boolean>();

const maxWidthClasses: Record<Size, string> = {
  default: "max-w-[450px]",
  lg: "max-w-[550px]",
};
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogTrigger as-child>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-30 bg-black/10 backdrop-blur-[5px]" />
      <DialogContent
        :class="[
          'bg-surface-subtle fixed top-[50%] left-[50%] z-[100] max-h-[85vh] w-[90vw] translate-x-[-50%] translate-y-[-50%] rounded-[14px] p-0.5 shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none',
          maxWidthClasses[size],
        ]"
      >
        <div class="bg-surface border-neutral rounded-xl border">
          <DialogTitle class="text-neutral border-neutral border-b px-4 py-3 text-sm font-medium">
            <slot name="title" />
          </DialogTitle>
          <DialogDescription class="text-neutral-subtle hidden">
            <slot name="description" />
          </DialogDescription>
          <div class="p-4">
            <slot name="content" />
          </div>
        </div>
        <div class="flex items-center justify-between p-2">
          <div>
            <slot name="footer-left" />
          </div>
          <div class="flex gap-2">
            <DialogClose as-child>
              <slot name="cancel" />
            </DialogClose>
            <slot name="submit" />
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
