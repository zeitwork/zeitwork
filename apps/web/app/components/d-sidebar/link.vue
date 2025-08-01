<script setup lang="ts">
const route = useRoute()
const orgSlug = computed<string>(() => route.params.org as string)
const projectSlug = computed<string>(() => route.params.project as string)

type Props = {
  to: string
  name: string
  active?: boolean
}
const { to, name } = defineProps<Props>()

const isActive = computed(() => {
  if (projectSlug.value) {
    return to === `/${orgSlug.value}/${projectSlug.value}` ? to === route.path : route.path.startsWith(to)
  }
  return to === `/${orgSlug.value}` ? to === route.path : route.path.startsWith(to)
})
</script>

<template>
  <NuxtLink
    class="block rounded-lg border px-3 py-2 text-sm"
    :class="[
      isActive
        ? 'border-neutral-200 bg-white text-neutral-700 shadow-sm'
        : 'border-transparent text-neutral-500 hover:bg-neutral-100',
    ]"
    :to="to"
  >
    {{ name }}
  </NuxtLink>
</template>
