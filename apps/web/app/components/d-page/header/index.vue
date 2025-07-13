<script lang="ts" setup>
import { LogOutIcon } from "lucide-vue-next"
type Props = {
  navigation: Array<{ name: string; to: string }>
}
const { navigation } = defineProps<Props>()
const route = useRoute()
const orgName = route.params.org

const me = ref({
  name: "Aaron Mahlke",
  organisationName: "Acme",
})

const initials = computed(() => {
  if (!me.value) return ""
  const name = me.value.name?.split(" ")
  return (name[0]?.charAt(0) || "") + (name[1]?.charAt(0) || "")
})
</script>

<template>
  <header class="bg-neutral border-neutral w-full border-b">
    <div class="flex items-center justify-between px-6 py-3">
      <div class="flex items-center gap-2">
        <NuxtLink :to="`/${orgName}`" class="flex flex-shrink-0 items-center gap-2">
          <DLogo class="h-5 text-black" />
        </NuxtLink>
        <DPageHeaderSeparator />
        <DPageHeaderBreadcrumbLink :name="me?.organisationName as string" :to="`/${orgName}`" />
        <template>
          <DPageHeaderSeparator />
          <DPageHeaderBreadcrumbLink name="Acme" :to="`/acme`" />
        </template>

        <template>
          <DPageHeaderSeparator />
          <DPageHeaderBreadcrumbLink name="Dokedu" :to="`/acme/dokedu`" />
        </template>
      </div>
      <div class="flex items-center gap-2">
        <DThemeSwitcher />
        <DButton :icon-left="LogOutIcon" variant="secondary" size="MD" to="/logout" />
        <div
          class="bg-neutral-inverse text-neutral-inverse text-copy-sm grid size-8 place-items-center rounded-full font-semibold uppercase"
        >
          {{ initials }}
        </div>
      </div>
    </div>

    <DPageHeaderNavigation :navigation="navigation" />
  </header>
</template>
