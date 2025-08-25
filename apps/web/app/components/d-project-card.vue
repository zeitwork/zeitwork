<script setup lang="ts">
import { GitMergeIcon, GithubIcon } from "lucide-vue-next"

const route = useRoute()

const orgName = route.params.org

type Props = {
  project: {
    id: string
    k8sName: string
    name: string
    organisationId: string
    githubOwner: string
    githubRepo: string
    port: number
  }
}

const { project } = defineProps<Props>()

// Construct GitHub URL from owner and repo
const githubUrl = computed(() => `https://github.com/${project.githubOwner}/${project.githubRepo}`)

// GitHub avatar URL
const githubAvatarUrl = computed(() => `https://github.com/${project.githubOwner}.png`)

// For now, we'll use placeholder data for fields not available in the API
// These could be fetched from GitHub API or stored separately in the future
const placeholderData = {
  commitMessage: "Latest commit",
  branch: "main",
  lastDeployDate: new Date(),
}

const formattedDate = computed(() => {
  const date = placeholderData.lastDeployDate
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  })
})
</script>
<template>
  <NuxtLink
    :to="`/${orgName}/${project.k8sName}`"
    class="bg-neutral border-neutral text-copy hover:border-neutral-strong/20 flex flex-col items-start gap-2 rounded-lg border p-4 shadow-sm transition-all hover:shadow"
  >
    <div class="flex items-center gap-2">
      <img
        :src="githubAvatarUrl"
        :alt="`${project.githubOwner}'s avatar`"
        class="border-neutral size-8 rounded-full border object-cover"
      />
      <div>
        <h2>{{ project.name }}</h2>
        <p class="text-neutral-subtle text-copy-sm">Port {{ project.port }}</p>
      </div>
    </div>
    <div class="bg-neutral-subtle text-copy-sm inline-flex items-center gap-1 rounded-full py-1 pr-3 pl-2">
      <GithubIcon class="size-4" />
      <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="hover:underline" @click.stop>
        {{ project.githubOwner }}/{{ project.githubRepo }}
      </a>
    </div>
    <p class="text-neutral-subtle line-clamp-1 text-sm">{{ placeholderData.commitMessage }}</p>
    <div class="text-neutral-subtle text-copy-sm flex items-center gap-1">
      <p>{{ formattedDate }}</p>
      <p>on</p>
      <GitMergeIcon class="size-4" />
      <p>{{ placeholderData.branch }}</p>
    </div>
  </NuxtLink>
</template>
