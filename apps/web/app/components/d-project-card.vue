<script setup lang="ts">
const route = useRoute()

const orgName = route.params.org

import { GitMergeIcon } from "lucide-vue-next"
import type { FragmentType } from "~/gql"
import type { Project_ProjectFragment } from "~/gql"

export type Project = FragmentType<typeof Project_ProjectFragment>

type Props = {
  project: Project
}

const { project } = defineProps<Props>()

const formattedDate = computed(() => {
  // const date = new Date(project.date)
  const date = new Date()
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  })
})
</script>
<template>
  <NuxtLink
    :to="`/${orgName}/${project.name}`"
    class="bg-neutral border-neutral text-copy hover:border-neutral-strong/20 flex flex-col items-start gap-2 rounded-lg border p-4 shadow-md hover:shadow-lg"
  >
    <div class="flex items-center gap-2">
      <div class="border-neutral size-8 rounded-full border"></div>
      <div>
        <h2>{{ project.name }}</h2>
        <p class="text-neutral-subtle text-copy-sm">{{ project.domain }}</p>
      </div>
    </div>
    <div class="bg-neutral-subtle text-copy-sm inline-flex items-center gap-1 rounded-full py-1 pr-3 pl-2">
      <img src="/icons/github-mark.svg" alt="GitHub Logo" class="size-4" />
      {{ project.githubUrl }}
    </div>
    <p class="line-clamp-1">{{ project.commitMessage }}</p>
    <div class="text-neutral-subtle text-copy-sm flex items-center gap-1">
      <p>{{ formattedDate }}</p>
      <p>on</p>
      <GitMergeIcon class="size-4" />
      <p>{{ project.branch }}</p>
    </div>
  </NuxtLink>
</template>
