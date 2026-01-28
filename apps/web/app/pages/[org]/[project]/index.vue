<script setup lang="ts">
import { GithubIcon } from "lucide-vue-next"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectSlug = route.params.project

const { data: project } = await useFetch(`/api/projects/${projectSlug}`)
const { data: environments } = await useFetch(`/api/projects/${projectSlug}/environments`)
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Project</div>
    </div>
    <div v-if="project" class="flex-1 overflow-auto p-4">
      <div class="flex flex-col gap-2">
        <div class="text-neutral text-sm">Environments</div>
        <div class="grid grid-cols-[200px_1fr] items-center gap-2">
          <NuxtLink
            v-for="environment in environments"
            :key="environment.id"
            :to="`/${route.params.org}/${route.params.project}/${environment.name}`"
            class="border-neutral flex min-h-32 min-w-64 flex-col justify-between gap-4 rounded-lg border p-4 hover:shadow"
          >
            <div class="flex items-center gap-2">
              <div class="text-neutral text-sm">{{ environment.name }}</div>
            </div>
            <div class="flex items-center gap-2">
              <div><GithubIcon class="text-neutral-subtle size-4" /></div>
              <div class="text-neutral-subtle text-copy-sm">{{ environment.branch }}</div>
            </div>
          </NuxtLink>
        </div>
      </div>
    </div>
  </div>
</template>
