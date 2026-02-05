<script setup lang="ts">
import { HourglassIcon } from "lucide-vue-next";

definePageMeta({
  layout: "default",
});
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Domains</div>
    </div>
    <div class="flex-1 overflow-auto">
      <div class="flex flex-col items-center justify-center gap-2 p-12">
        <div>
          <HourglassIcon class="text-neutral-subtle size-12 stroke-[1.5]" />
        </div>
        <div class="text-neutral-subtle font-medium">Coming soon</div>
        <div class="text-neutral-weak text-sm">
          We are working on adding this feature to your account.
        </div>
      </div>
    </div>
  </div>
</template>
