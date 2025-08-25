<script setup lang="ts">
definePageMeta({
  layout: "project",
})

const route = useRoute()
const orgId = route.params.org
const projectId = route.params.project

const { data: project, refresh } = await useFetch(`/api/organisations/${orgId}/projects/${projectId}`)

const lastDeployUrl = computed(() => {
  return `https://${project.value?.latestDeploymentURL}`
})

const domains = computed(() => {
  return "app.example.com"
})

const domain = ref("")
const isEditingDomain = ref(false)
const envVariables = ref<Array<{ name: string; value: string }>>([])
const isSavingEnv = ref(false)
const envSaveError = ref<string | null>(null)

async function addDomain() {
  await $fetch(`/api/organisations/${orgId}/projects/${projectId}`, {
    method: "PATCH",
    body: { domain: domain.value },
  })
  await refresh()
  domain.value = ""
  isEditingDomain.value = false
}

async function updateDomain() {
  await $fetch(`/api/organisations/${orgId}/projects/${projectId}`, {
    method: "PATCH",
    body: { domain: domain.value },
  })
  await refresh()
  domain.value = ""
  isEditingDomain.value = false
}

function startEditingDomain() {
  domain.value = project.value?.domain || ""
  isEditingDomain.value = true
}

function cancelEditingDomain() {
  domain.value = ""
  isEditingDomain.value = false
}

async function saveEnvVariables() {
  isSavingEnv.value = true
  envSaveError.value = null
  try {
    await $fetch(`/api/organisations/${orgId}/projects/${projectId}`, {
      method: "PATCH",
      body: {
        env: envVariables.value.filter((e) => e.name && e.value),
      },
    })
    await refresh()
    // Note: Environment variables are saved but not yet reflected in the project data
    // as the backend doesn't return them in the GET response yet
  } catch (error) {
    console.error("Failed to save environment variables:", error)
    envSaveError.value = "Failed to save environment variables. Please try again."
  } finally {
    isSavingEnv.value = false
  }
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
    <div v-if="project">
      <div class="border-neutral grid grid-cols-[1fr_2fr] gap-4 rounded-lg border bg-white p-4">
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
              <div v-if="project.domain && !isEditingDomain" class="flex items-center gap-2">
                <NuxtLink :to="`https://${project.domain}`" target="_blank" external>
                  {{ project.domain }}
                </NuxtLink>
                <DButton variant="secondary" size="sm" @click="startEditingDomain">Edit</DButton>
              </div>
              <div v-else-if="isEditingDomain" class="flex gap-2">
                <input
                  v-model="domain"
                  placeholder="example.com"
                  type="text"
                  class="border-neutral text-neutral rounded-md border px-2.5 py-1.5 text-sm"
                />
                <DButton variant="secondary" size="sm" @click="updateDomain">Update</DButton>
                <DButton variant="secondary" size="sm" @click="cancelEditingDomain">Cancel</DButton>
              </div>
              <div v-else class="flex gap-2">
                <input
                  v-model="domain"
                  placeholder="example.com"
                  type="text"
                  class="border-neutral text-neutral rounded-md border px-2.5 py-1.5 text-sm"
                />
                <DButton variant="secondary" size="sm" @click="addDomain">Add Domain</DButton>
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
        <div>
          <DButton :loading="isDeployingLatestCommit" variant="secondary" size="sm" @click="deployLatestCommit">
            Deploy latest commit
          </DButton>
        </div>
      </div>
    </div>
  </div>
</template>
