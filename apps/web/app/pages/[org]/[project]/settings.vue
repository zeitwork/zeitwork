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
const envVariables = ref<Array<{ name: string; value: string }>>([])
const isSavingEnv = ref(false)
const envSaveError = ref<string | null>(null)

async function addDomain() {
  await $fetch(`/api/organisations/${orgId}/projects/${projectId}`, {
    method: "PATCH",
    body: { domain: domain.value },
  })
  await refresh()
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
    <div>
      <h2 class="mb-4 text-lg font-semibold">Environment Variables</h2>
      <div class="border-neutral rounded-lg border bg-white p-4">
        <DEnvVariables v-model="envVariables" />
        <div v-if="envSaveError" class="mt-4 text-sm text-red-600">
          {{ envSaveError }}
        </div>
        <div class="mt-4">
          <DButton :loading="isSavingEnv" variant="primary" size="md" @click="saveEnvVariables">
            Save Environment Variables
          </DButton>
        </div>
      </div>
    </div>
  </div>
</template>
