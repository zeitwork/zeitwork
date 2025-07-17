<script setup lang="ts">
const route = useRoute()
const orgSlug = computed<string>(() => route.params.org as string)

// Fetch organisation by slug
const { data: organisation } = await useFetch(`/api/organisations/${orgSlug.value}`)

// Fetch projects for this organisation
const { data: projects } = await useFetch(`/api/organisations/${organisation.value?.id}/projects`, {
  // Only fetch if organisation exists
  immediate: !!organisation.value?.id,
})

// Check if GitHub App is installed
const hasGitHubInstallation = computed(() => !!organisation.value?.installationId)

// GitHub App installation URL
const githubAppInstallUrl = computed(() => {
  const appName = "zeitwork" // Your GitHub App name
  return `https://github.com/apps/${appName}/installations/new?state=${orgSlug.value}`
})

// Check for installation success message
const justInstalled = computed(() => route.query.installed === "true")
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <!-- Success message after installation -->
      <div
        v-if="justInstalled"
        style="background: #d4edda; border: 1px solid #c3e6cb; padding: 12px; border-radius: 4px"
      >
        GitHub App installed successfully! You can now create projects.
      </div>

      <!-- GitHub App installation prompt -->
      <div
        v-if="!hasGitHubInstallation"
        style="background: #fff3cd; border: 1px solid #ffeaa7; padding: 16px; border-radius: 4px"
      >
        <h3>Install GitHub App Required</h3>
        <p>To create and deploy projects, you need to install the Zeitwork GitHub App for this organisation.</p>
        <a
          :href="githubAppInstallUrl"
          style="
            display: inline-block;
            margin-top: 8px;
            padding: 8px 16px;
            background: #24292e;
            color: white;
            text-decoration: none;
            border-radius: 4px;
          "
        >
          Install GitHub App
        </a>
      </div>

      <div class="flex gap-2">
        <DInput class="flex-1" placeholder="Search Projects..." />
        <DButton :to="`/${orgSlug}/new`" :disabled="!hasGitHubInstallation">Add Project</DButton>
      </div>

      <div v-if="projects && projects.length > 0" class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <pre>{{ projects }}</pre>
        <!-- <DProjectCard v-for="project in projects" :key="project.id" :project="project" /> -->
      </div>

      <div v-else-if="hasGitHubInstallation">
        <p>No projects yet. Create your first project!</p>
      </div>
    </div>
  </DPageWrapper>
</template>
