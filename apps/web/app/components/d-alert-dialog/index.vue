<script setup lang="ts">
import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "reka-ui";

type Size = "md" | "lg";

type Props = {
  size?: Size;
};

const { size = "md" } = defineProps<Props>();
const open = defineModel<boolean>();

const maxWidthClasses: Record<Size, string> = {
  md: "max-w-[450px]",
  lg: "max-w-[550px]",
};
</script>

<template>
  <AlertDialogRoot v-model:open="open">
    <AlertDialogTrigger as-child>
      <slot name="trigger" />
    </AlertDialogTrigger>
    <AlertDialogPortal>
      <AlertDialogOverlay class="fixed inset-0 z-30 bg-overlay backdrop-blur-[2px]" />
      <AlertDialogContent
        :class="[
          'base-2 fixed top-[50%] left-[50%] z-[100] max-h-[85vh] w-[90vw] translate-x-[-50%] translate-y-[-50%] rounded-[14px] p-0.5 shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none',
          maxWidthClasses[size],
        ]"
      >
        <div class="base-3 border-edge rounded-xl border">
          <AlertDialogTitle class="text-primary border-edge border-b px-4 py-3 text-sm font-medium">
            <slot name="title" />
          </AlertDialogTitle>
          <AlertDialogDescription as-child>
            <div class="p-4">
              <slot name="content" />
            </div>
          </AlertDialogDescription>
        </div>
        <div class="flex items-center justify-end gap-2 p-2">
          <AlertDialogCancel as-child>
            <slot name="cancel" />
          </AlertDialogCancel>
          <AlertDialogAction as-child>
            <slot name="action" />
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>
