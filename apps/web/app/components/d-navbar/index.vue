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
  <div class="flex w-screen flex-row py-2">
    <div class="flex flex-row items-center px-6">
      <NuxtLink :to="`/${orgSlug}`" class="flex items-center gap-4">
        <d-logo />
        <div class="text-sm text-neutral-500">{{ orgSlug }}</div>
      </NuxtLink>
    </div>
    <div class="flex flex-row gap-2 px-3">
      <d-navbar-link v-for="link in links" :key="link.name" :to="link.to" :name="link.name" />
    </div>
  </div>
</template>
