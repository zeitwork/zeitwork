<script setup lang="ts">
import { useBrowserLocation } from "@vueuse/core"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectSlug = route.params.project as string
</script>

<template>
  <div class="flex h-full flex-row overflow-auto">
    <div class="border-neutral-subtle flex w-64 flex-col gap-2 border-r p-4">
      <DButton
        :to="`/${orgId}/${projectSlug}/settings`"
        :variant="route.path === `/${orgId}/${projectSlug}/settings` ? 'primary' : 'secondary'"
        class="w-full"
      >
        General
      </DButton>
      <DButton
        :to="`/${orgId}/${projectSlug}/settings/domains`"
        :variant="route.path === `/${orgId}/${projectSlug}/settings/domains` ? 'primary' : 'secondary'"
        class="w-full"
      >
        Domains
      </DButton>
    </div>
    <div class="flex-1 overflow-auto"></div>
  </div>
</template>
