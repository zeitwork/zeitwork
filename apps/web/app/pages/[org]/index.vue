<script setup lang="ts">
definePageMeta({
  layoutTransition: {
    name: "layout-forward",
    mode: "out-in",
  },
});

const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);

// Search query
const searchQuery = ref("");

const filteredProjects = computed(() => {
  if (!projects.value) return [];
  const query = searchQuery.value.toLowerCase().trim();
  if (!query) return projects.value;
  return projects.value.filter(
    (project) =>
      project.name.toLowerCase().includes(query) ||
      project.githubRepository.toLowerCase().includes(query),
  );
});

// Fetch organisation by slug
const { data: organisation } = await useFetch(`/api/organisations/${orgSlug.value}`);

const { data: projects } = await useFetch(`/api/projects`);

// Check if GitHub App is installed
const hasGitHubInstallation = computed(() => !!organisation.value?.installationId);

const config = useRuntimeConfig();

// GitHub App installation URL
const githubAppInstallUrl = computed(() => {
  const appName = "zeitwork"; // Your GitHub App name
  const redirectUri = `${config.appUrl}/auth/github`; // in prod: https://zeitwork.com/auth/github
  return `https://github.com/apps/${appName}/installations/new?redirect_uri=${redirectUri}`;
});

// Check for installation success message
const justInstalled = computed(() => route.query.installed === "true");
</script>

<template>
  <div class="@container flex flex-col gap-4 p-4">
    <div class="flex gap-2">
      <DInput v-model="searchQuery" class="flex-1" placeholder="Search Projects..." />
      <!-- <DButton :to="`/${orgSlug}/new`" :disabled="!hasGitHubInstallation">Add Project</DButton> -->
      <DButton :to="`/${orgSlug}/new`">Add Project</DButton>
    </div>

    <div
      v-if="filteredProjects.length > 0"
      class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4"
    >
      <DProjectCard v-for="project in filteredProjects" :key="project.id" :project="project" />
    </div>

    <div v-else-if="projects && projects.length > 0 && filteredProjects.length === 0">
      <p class="text-secondary text-copy-sm">No projects match your search.</p>
    </div>

    <div v-else-if="!projects || projects.length === 0">
      <p class="text-secondary text-copy-sm">No projects yet. Create your first project!</p>
    </div>
  </div>
</template>
