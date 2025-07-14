<script setup lang="ts">
type Props = {
  navigation: Array<{ name: string; to: string }>
  padding?: boolean
}

const { navigation, padding = true } = defineProps<Props>()
const route = useRoute()
</script>

<template>
  <div class="bg-neutral px-2" :class="[padding ? 'px-2 md:px-4' : '']">
    <div class="flex overflow-x-auto whitespace-nowrap no-scrollbar md:overflow-visible md:whitespace-normal">
      <NuxtLink
        v-for="item in navigation"
        :key="item?.name"
        :to="item?.to"
        :class="
          [
            'mx-1', // horizontal spacing
            'rounded-md', // more button-like
            'transition duration-150 ease-in-out',
            'text-sm md:text-base', // smaller font on mobile
            'px-2 py-1 md:px-3 md:py-2', // smaller padding on mobile
            'bg-neutral-50 md:bg-transparent', // subtle background on mobile
            'border border-neutral-200 md:border-b-2 md:border-neutral-200', // border for touch target
            route.path.endsWith(item?.to)
              ? 'text-neutral border-neutral-strong md:border-neutral-strong bg-neutral-100'
              : 'text-neutral-subtle border-transparent'
          ]
        "
        class="group hover:text-neutral focus:bg-neutral-100 md:hover:bg-neutral-subtle"
      >
        <div class="group-hover:bg-neutral-subtle text-copy rounded-lg px-1 md:px-3 py-0.5 md:py-1">
          {{ item?.name }}
        </div>
      </NuxtLink>
    </div>
  </div>
</template>
