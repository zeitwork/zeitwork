<script setup lang="ts">
type Props = {
  navigation: Array<{ name: string; to: string }>
  padding?: boolean
}

const { navigation, padding = true } = defineProps<Props>()
const route = useRoute()
</script>

<template>
  <div class="bg-neutral" :class="[padding ? 'px-4' : '']">
    <div class="flex">
      <NuxtLink
        v-for="item in navigation"
        :key="item?.name"
        :to="item?.to"
        :class="
          route.path.endsWith(item?.to)
            ? 'text-neutral border-neutral-strong'
            : 'text-neutral-subtle border-transparent'
        "
        class="group hover:text-neutral border-b-2 py-2 transition duration-150 ease-in-out"
      >
        <div class="group-hover:bg-neutral-subtle text-copy rounded-lg px-3 py-1">
          {{ item?.name }}
        </div>
      </NuxtLink>
    </div>
  </div>
</template>
