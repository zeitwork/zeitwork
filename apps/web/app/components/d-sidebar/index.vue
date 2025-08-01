<script setup lang="ts">
const route = useRoute()
const orgSlug = computed<string>(() => route.params.org as string)

interface Link {
  name: string
  to: string
}

interface Props {
  links: Link[]
}

const { links } = defineProps<Props>()
</script>

<template>
  <div class="h-screen w-[300px]">
    <div class="flex h-14 items-center px-6">
      <NuxtLink :to="`/${orgSlug}`" class="flex items-center gap-4">
        <d-logo />
        <div class="text-sm text-neutral-500">{{ orgSlug }}</div>
      </NuxtLink>
    </div>
    <div class="flex flex-col gap-2 px-3">
      <d-sidebar-link v-for="link in links" :key="link.name" :to="link.to" :name="link.name" />
    </div>
  </div>
</template>
