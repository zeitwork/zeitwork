<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectId = route.params.project

const { data: project } = await useFetch(`/api/organisations/${orgId}/projects/${projectId}`)

const lastDeployUrl = computed(() => {
  return `https://${project.value?.latestDeploymentURL}`
})

const domains = computed(() => {
  return "app.example.com"
})
</script>

<template>
  <DPageWrapper>
    <DPageTitle :title="project?.name ?? 'Project'"></DPageTitle>
    <div v-if="project">
      <div class="border-neutral mt-4 grid grid-cols-[1fr_2fr] gap-4 rounded-lg border bg-white p-4">
        <div class="bg-neutral-subtle h-[300px] w-full rounded-md"></div>
        <div class="flex flex-col gap-2.5">
          <div class="flex flex-col gap-1.5">
            <div class="text-neutral-subtle text-sm font-medium">Latest Deployment</div>
            <div class="text-neutral">
              <NuxtLink :to="lastDeployUrl" target="_blank" external>
                {{ project.latestDeploymentURL }}
              </NuxtLink>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <div class="text-neutral-subtle text-sm font-medium">Domain</div>
            <div class="text-neutral">
              <NuxtLink v-if="project.domain" :to="`https://${project.domain}`" target="_blank" external>
                {{ project.domain }}
              </NuxtLink>
              <div v-else>
                <DButton variant="secondary" size="sm">Add Domain</DButton>
              </div>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <div class="text-neutral-subtle text-sm font-medium">Repository</div>
            <div class="text-neutral">
              <NuxtLink
                :to="`https://github.com/${project.githubOwner}/${project.githubRepo}`"
                target="_blank"
                external
              >
                {{ project.githubOwner }}/{{ project.githubRepo }}
              </NuxtLink>
            </div>
          </div>
        </div>
      </div>
    </div>
  </DPageWrapper>
</template>
