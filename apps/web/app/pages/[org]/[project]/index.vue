<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectSlug = route.params.project

const { data: project, refresh } = await useFetch(`/api/projects/${projectSlug}`)
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="border-neutral-subtle flex h-16 items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Project</div>
      <!-- <d-button @click="createDeployment">Create Deployment</d-button> -->
    </div>
    <div v-if="project" class="flex-1 overflow-auto">
      <!-- <div
        v-for="deployment in deployments"
        :key="deployment.id"
        class="hover:bg-surface-subtle border-neutral-subtle text-neutral grid grid-cols-[100px_100px_100px_3fr_1fr] items-center gap-2 border-b p-4 text-sm"
      >
        <div class="font-mono">{{ deployment.deploymentId }}</div>
        <div>
          <div
            :class="[
              deploymentStatusColor(deployment.status),
              deploymentStatusBgColor(deployment.status),
              'w-fit rounded px-1.5 py-0.5',
            ]"
          >
            {{ deployment.status }}
          </div>
        </div>
        <div class="font-mono">{{ deployment.githubCommit.slice(0, 7) }}</div>
        <div>
          <nuxt-link :to="formatDeploymentUrl(deployment)" external target="_blank">
            {{ deployment.domains?.[0]?.name }}
          </nuxt-link>
        </div>
        <div class="text-right">{{ renderDate(deployment.createdAt) }}</div>
      </div>
    </div> -->
      <div
        v-for="fieldKey in Object.keys(project)"
        class="hover:bg-surface-subtle border-neutral-subtle text-neutral grid grid-cols-[200px_1fr] items-center gap-2 border-b p-4 text-sm"
      >
        <div>{{ fieldKey }}</div>
        <div class="text-neutral-subtle">{{ project[fieldKey] }}</div>
      </div>
    </div>
  </div>
</template>
