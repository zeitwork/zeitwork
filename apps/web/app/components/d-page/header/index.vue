<script lang="ts" setup>
import { LogOutIcon } from "lucide-vue-next"
type Props = {
  navigation: Array<{ name: string; to: string }>
}
const { navigation } = defineProps<Props>()
const route = useRoute()
const orgName = route.params.org

const projectName = route.params.project

const { user, clear } = useUserSession()

const me = computed(() => user.value)

const initials = computed(() => {
  if (!me.value) return ""
  const name = me.value.name?.split(" ")
  return (name[0]?.charAt(0) || "") + (name[1]?.charAt(0) || "")
})

async function logout() {
  await clear()
  await navigateTo("/login")
}
</script>

<template>
  <header class="bg-neutral border-neutral w-full border-b">
    <div class="flex items-center justify-between px-6 py-4">
      <div class="flex items-center gap-2">
        <NuxtLink :to="`/${orgName}`" class="flex flex-shrink-0 items-center gap-2">
          <DLogo class="h-5 text-black" />
        </NuxtLink>
        <DPageHeaderSeparator />
        <DPageHeaderBreadcrumbLink :name="orgName as string" :to="`/${orgName}`" />

        <template v-if="projectName">
          <DPageHeaderSeparator />
          <DPageHeaderBreadcrumbLink :name="projectName as string" :to="`/${orgName}/${projectName}`" />
        </template>
      </div>
      <div class="flex items-center gap-2">
        <!-- <DThemeSwitcher /> -->
        <DButton :icon-left="LogOutIcon" variant="secondary" size="MD" @click="logout" />
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
