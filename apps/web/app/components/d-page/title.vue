<script setup lang="ts">
type Props = {
  title: string
  wide?: boolean
  size?: "sm" | "md"
}

const { title, wide = false, size = "md" } = defineProps<Props>()

const height = computed(() => {
  switch (size) {
    case "sm":
      return "h-20"
    case "md":
      return "h-30"
    default:
      return "h-30"
  }
})
</script>
<template>
  <div class="border-neutral border-b">
    <DPageWrapper :wide>
      <div class="flex items-center justify-between gap-10" :class="[height]">
        <div class="flex-1">
          <div class="text-neutral-strong line-clamp-1" :class="[size === 'sm' ? 'text-title-sm' : 'text-title']">
            {{ title }}
          </div>
          <slot name="subtitle" />
        </div>
        <slot name="center" />
        <div class="flex flex-1 justify-end">
          <slot />
        </div>
      </div>
    </DPageWrapper>
  </div>
</template>
