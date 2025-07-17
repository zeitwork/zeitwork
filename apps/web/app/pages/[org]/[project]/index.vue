<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectId = route.params.project

const { data: project } = await useFetch(`/api/organisations/${orgId}/projects/${projectId}`)
</script>

<template>
  <DPageWrapper>
    <DPageTitle title="Project"> </DPageTitle>
    <div v-if="project" class="space-y-4">
      <div>
        <h3 class="text-sm font-medium text-gray-500">Repository</h3>
        <p class="mt-1 text-sm text-gray-900">{{ project.githubOwner }}/{{ project.githubRepo }}</p>
      </div>

      <div>
        <h3 class="text-sm font-medium text-gray-500">Port</h3>
        <p class="mt-1 text-sm text-gray-900">{{ project.port }}</p>
      </div>

      <div v-if="project.latestDeploymentURL">
        <h3 class="text-sm font-medium text-gray-500">Latest Deployment</h3>
        <a
          :href="`https://${project.latestDeploymentURL}`"
          target="_blank"
          class="mt-1 text-sm text-blue-600 hover:text-blue-500"
        >
          {{ project.latestDeploymentURL }}
        </a>
      </div>

      <div v-else>
        <h3 class="text-sm font-medium text-gray-500">Latest Deployment</h3>
        <p class="mt-1 text-sm text-gray-500 italic">No deployments yet</p>
      </div>
    </div>
  </DPageWrapper>
</template>
