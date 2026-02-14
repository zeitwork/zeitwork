<script setup lang="ts">
import { GitCommitHorizontalIcon, GithubIcon } from "lucide-vue-next";

const route = useRoute();

const orgName = route.params.org;

interface Project {
  id: string;
  name: string;
  slug: string;
  githubRepository: string;
  organisationId: string;
  createdAt: string;
  updatedAt: string;
  deletedAt: string;
  latestDeploymentCommit: string | null;
  latestDeploymentDate: string | null;
}

type Props = {
  project: Project;
};

const { project } = defineProps<Props>();

const githubOwner = computed(
  () => project.githubRepository.split("/")[0] ?? "",
);
const githubRepo = computed(() => project.githubRepository.split("/")[1] ?? "");

// Construct GitHub URL from owner and repo
const githubUrl = computed(
  () => `https://github.com/${githubOwner.value}/${githubRepo.value}`,
);

// GitHub avatar URL
const githubAvatarUrl = computed(
  () => `https://github.com/${githubOwner.value}.png`,
);

const shortCommitHash = computed(() => {
  if (!project.latestDeploymentCommit) return null;
  return project.latestDeploymentCommit.substring(0, 7);
});

const commitUrl = computed(() => {
  if (!project.latestDeploymentCommit) return null;
  return `https://github.com/${project.githubRepository}/commit/${project.latestDeploymentCommit}`;
});

const formattedDate = computed(() => {
  if (!project.latestDeploymentDate) return null;
  const date = new Date(project.latestDeploymentDate);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
});
</script>

<template>
  <NuxtLink
    :to="`/${orgName}/${project.slug}`"
    class="base-3 border-edge text-copy hover:border-edge-strong flex flex-col items-start gap-2 rounded-lg border p-4 shadow-sm transition-all hover:shadow"
  >
    <div class="flex items-center gap-2">
      <img
        :src="githubAvatarUrl"
        :alt="`${githubOwner}'s avatar`"
        class="border-edge size-8 rounded-full border object-cover"
      />
      <h2>{{ project.name }}</h2>
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
    <div
      v-if="shortCommitHash"
      class="text-secondary text-copy-sm flex items-center gap-1"
    >
      <GitCommitHorizontalIcon class="size-4" />
      <a
        :href="commitUrl!"
        target="_blank"
        rel="noopener noreferrer"
        class="font-mono hover:underline"
        @click.stop
      >
        {{ shortCommitHash }}
      </a>
    </div>
    <p v-else class="text-tertiary text-copy-sm">No deployments yet</p>
    <p v-if="formattedDate" class="text-secondary text-copy-sm">
      {{ formattedDate }}
    </p>
  </NuxtLink>
</template>
