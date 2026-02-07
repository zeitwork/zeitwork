<script setup lang="ts">
import { GitMergeIcon, GithubIcon } from "lucide-vue-next";

const route = useRoute();

const orgName = route.params.org;

interface Project {
  id: string;
  name: string;
  slug: string;
  githubRepository: string;
  defaultBranch: string;
  latestDeploymentId: string;
  organisationId: string;
  createdAt: string;
  updatedAt: string;
  deletedAt: string;
}

type Props = {
  project: Project;
};

const { project } = defineProps<Props>();

const githubOwner = computed(() => project.githubRepository.split("/")[0] ?? "");
const githubRepo = computed(() => project.githubRepository.split("/")[1] ?? "");

// Construct GitHub URL from owner and repo
const githubUrl = computed(() => `https://github.com/${githubOwner.value}/${githubRepo.value}`);

// GitHub avatar URL
const githubAvatarUrl = computed(() => `https://github.com/${githubOwner.value}.png`);

// For now, we'll use placeholder data for fields not available in the API
// These could be fetched from GitHub API or stored separately in the future
const placeholderData = {
  commitMessage: "Latest commit",
  branch: "main",
  lastDeployDate: new Date(),
};

const formattedDate = computed(() => {
  const date = placeholderData.lastDeployDate;
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
});
</script>

<template>
  <NuxtLink
    :to="`/${orgName}/${project.slug}`"
    class="bg-surface-0 border-edge text-copy hover:border-edge-strong flex flex-col items-start gap-2 rounded-lg border p-4 shadow-sm transition-all hover:shadow"
  >
    <div class="flex items-center gap-2">
      <img
        :src="githubAvatarUrl"
        :alt="`${githubOwner}'s avatar`"
        class="border-edge size-8 rounded-full border object-cover"
      />
      <div>
        <h2>{{ project.name }}</h2>
        <p class="text-secondary text-copy-sm">Port {{ project.latestDeploymentId }}</p>
      </div>
    </div>
    <div
      class="bg-surface-1 text-copy-sm inline-flex items-center gap-1 rounded-full py-1 pr-3 pl-2"
    >
      <GithubIcon class="size-4" />
      <a
        :href="githubUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="hover:underline"
        @click.stop
      >
        {{ githubOwner }}/{{ githubRepo }}
      </a>
    </div>
    <p class="text-secondary line-clamp-1 text-sm">{{ placeholderData.commitMessage }}</p>
    <div class="text-secondary text-copy-sm flex items-center gap-1">
      <p>{{ formattedDate }}</p>
      <p>on</p>
      <GitMergeIcon class="size-4" />
      <p>{{ placeholderData.branch }}</p>
    </div>
  </NuxtLink>
</template>
