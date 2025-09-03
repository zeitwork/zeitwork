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
  <NuxtLink class="group relative flex h-8 items-center px-3 text-sm" :to="to">
    <div
      class="bg-surface-strong absolute rounded-lg transition-all group-active:inset-[-1px]"
      :class="[isActive ? 'inset-0 opacity-100' : 'inset-1 opacity-0 group-hover:inset-0 group-hover:opacity-100']"
    ></div>
    <div
      class="z-10 transition-all"
      :class="[isActive ? 'text-neutral' : 'text-neutral-subtle group-hover:text-neutral']"
    >
      {{ name }}
    </div>
  </NuxtLink>
</template>
