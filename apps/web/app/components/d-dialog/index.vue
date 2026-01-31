<script setup lang="ts">
import { Icon } from "@iconify/vue";
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
</script>

<template>
  <DialogRoot>
    <DialogTrigger>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-30 bg-black/10" />
      <DialogContent
        class="fixed top-[50%] left-[50%] z-[100] max-h-[85vh] w-[90vw] max-w-[450px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
      >
        <DialogTitle class="m-0 text-[17px] font-semibold">
          <slot name="title" />
        </DialogTitle>
        <DialogDescription class="mt-[10px] mb-5 text-sm leading-normal">
          <slot name="description" />
        </DialogDescription>
        <slot name="content" />
        <div class="mt-6 flex justify-between">
          <DialogClose as-child>
            <slot name="cancel" />
          </DialogClose>
          <DialogClose as-child>
            <slot name="submit" />
          </DialogClose>
        </div>
        <DialogClose
          class="absolute top-[10px] right-[10px] inline-flex h-[25px] w-[25px] appearance-none items-center justify-center rounded-full focus:shadow-[0_0_0_2px] focus:outline-none"
          aria-label="Close"
        >
          <slot name="close-x" />
        </DialogClose>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
