<script setup lang="ts">
import { ExternalLinkIcon, GitCommitIcon, ClockIcon, CheckCircleIcon } from "lucide-vue-next"

definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org as string
const projectId = route.params.project as string

// Fetch organisation to get its ID
const { data: organisation } = await useFetch(`/api/organisations/${orgId}`)

// Fetch project details
const { data: project } = await useFetch(`/api/organisations/${orgId}/projects/${projectId}`)

// Fetch deployments for this project
const {
  data: deployments,
  pending,
  refresh,
} = await useFetch(`/api/organisations/${organisation.value?.id}/projects/${projectId}/deployments`, {
  // Only fetch if organisation exists
  immediate: !!organisation.value?.id,
})

// Search/filter query
const searchQuery = ref("")

// Filtered deployments based on search
const filteredDeployments = computed(() => {
  if (!deployments.value || !searchQuery.value.trim()) {
    return deployments.value || []
  }

  const query = searchQuery.value.toLowerCase().trim()
  return deployments.value.filter((deployment) => {
    return deployment.id.toLowerCase().includes(query) || deployment.previewURL.toLowerCase().includes(query)
  })
})

// Helper to format deployment ID (show first 7 chars of commit SHA)
const formatDeploymentId = (id: string) => {
  // Extract the commit SHA from deployment ID (format: sha-projectname-random)
  const parts = id.split("-")
  if (parts.length > 0 && parts[0]) {
    return parts[0].substring(0, 7)
  }
  return id.substring(0, 7)
}

// Helper to format deployment time (mock data for now)
const formatDeploymentTime = (deployment: any) => {
  // In a real app, this would come from deployment metadata
  const date = new Date()
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    hour12: true,
  })
}

// Helper to get deployment status (mock data for now)
const getDeploymentStatus = (deployment: any) => {
  // In a real app, this would come from deployment metadata
  return "success"
}

// Get deployment preview URL with https
const getDeploymentUrl = (previewURL: string) => {
  return `https://${previewURL}`
}

const isDeployingLatestCommit = ref(false)

async function deployLatestCommit() {
  isDeployingLatestCommit.value = true
  try {
    await $fetch(`/api/organisations/${orgId}/projects/${projectId}/deploy`, {
      method: "POST",
    })
    await refresh()
  } catch (error) {
    console.error("Failed to deploy latest commit:", error)
  }
  isDeployingLatestCommit.value = false
}
</script>

<template>
  <div class="p-4">
    <!-- <DPageTitle :title="`${project?.name} Deployments`" size="sm">
      <template #subtitle>
        <p class="text-neutral-subtle text-copy">
          Deployments are automatically created from commits to your GitHub repository
        </p>
      </template>
      <DButton @click="refresh" :loading="pending">Refresh</DButton>
    </DPageTitle> -->

    <div class="flex flex-col gap-4">
      <!-- Search bar -->
      <div class="flex w-full items-center justify-between gap-2">
        <DInput v-model="searchQuery" placeholder="Search deployments by ID or URL..." class="w-full max-w-lg" />
        <DButton @click="deployLatestCommit" :loading="isDeployingLatestCommit">Deploy latest commit</DButton>
      </div>

      <!-- Deployments list -->
      <div v-if="pending && !deployments" class="py-12 text-center">
        <p class="text-neutral-subtle">Loading deployments...</p>
      </div>

      <div v-else-if="filteredDeployments && filteredDeployments.length > 0">
        <DList>
          <DListItem v-for="deployment in filteredDeployments" :key="deployment.id" :padding="false">
            <div class="flex w-full items-center justify-between px-6 py-4">
              <!-- Left side: Deployment info -->
              <div class="flex items-center gap-6">
                <!-- Status icon -->
                <div class="flex-shrink-0">
                  <CheckCircleIcon v-if="getDeploymentStatus(deployment) === 'success'" class="size-5 text-green-600" />
                  <ClockIcon v-else class="size-5 text-yellow-600" />
                </div>

                <!-- Deployment details -->
                <div class="flex flex-col gap-2">
                  <div class="flex items-center gap-3">
                    <div class="flex items-center gap-1.5">
                      <GitCommitIcon class="text-neutral-subtle size-4" />
                      <span class="font-mono text-sm font-medium">{{ formatDeploymentId(deployment.id) }}</span>
                    </div>
                    <span class="text-neutral-subtle text-sm">•</span>
                    <span class="text-neutral-subtle text-sm">{{ formatDeploymentTime(deployment) }}</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <a
                      :href="getDeploymentUrl(deployment.previewURL)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="flex items-center gap-1.5 text-sm text-blue-600 hover:text-blue-700 hover:underline"
                      @click.stop
                    >
                      {{ deployment.previewURL }}
                      <ExternalLinkIcon class="size-3.5" />
                    </a>
                  </div>
                </div>
              </div>

              <!-- Right side: Actions -->
              <div class="flex items-center gap-3">
                <DButton variant="secondary" size="sm" :to="getDeploymentUrl(deployment.previewURL)" target="_blank">
                  Visit
                </DButton>
              </div>
            </div>
          </DListItem>
        </DList>
      </div>

      <div v-else-if="searchQuery && deployments && deployments.length > 0">
        <div class="py-12 text-center">
          <p class="text-neutral-subtle">No deployments match your search</p>
        </div>
      </div>

      <div v-else>
        <div class="py-12 text-center">
          <p class="text-neutral-subtle mb-4">No deployments yet</p>
          <p class="text-neutral-subtle text-sm">Push a commit to your repository to trigger your first deployment</p>
        </div>
      </div>
    </div>
  </div>
</template>
