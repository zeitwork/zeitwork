<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectSlug = route.params.project as string
</script>

<template>
  <div class="flex h-full shrink-0 flex-row overflow-auto">
    <div class="border-neutral-subtle flex w-50 flex-col gap-1 border-r p-2">
      <DNavbarLink name="General" :to="`/${orgId}/${projectSlug}/settings/general`"></DNavbarLink>
      <DNavbarLink name="Domains" :to="`/${orgId}/${projectSlug}/settings/domains`"></DNavbarLink>
    </div>
    <div class="flex flex-1 flex-col overflow-auto">
      <div class="flex-1 overflow-auto">
        <NuxtPage />
      </div>
    </div>
  </div>
</template>
