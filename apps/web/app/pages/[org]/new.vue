<script setup lang="ts">
const route = useRoute()
const router = useRouter()
const orgId = computed<string>(() => route.params.org as string)

// Form state
const formData = reactive({
  name: "",
  githubOwner: "",
  githubRepo: "",
  port: 3000,
  desiredRevisionSHA: "",
})

const error = ref<string | null>(null)
const isSubmitting = ref(false)

// Handle form submission
async function handleSubmit(event: Event) {
  event.preventDefault()
  error.value = null
  isSubmitting.value = true

  try {
    const response = await $fetch(`/api/organisations/${orgId.value}/projects`, {
      method: "POST",
      body: {
        name: formData.name,
        githubOwner: formData.githubOwner,
        githubRepo: formData.githubRepo,
        port: formData.port,
        desiredRevisionSHA: formData.desiredRevisionSHA || undefined,
      },
    })

    // Redirect to project page on success
    await router.push(`/${orgId.value}/${response.id}`)
  } catch (err: any) {
    error.value = err.data?.message || "Failed to create project"
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <h1 class="text-title-md">Create New Project</h1>

      <form
        @submit="handleSubmit"
        class="bg-neutral border-neutral flex flex-col gap-4 rounded-lg border p-4 shadow-sm"
      >
        <div class="flex flex-col gap-1.5">
          <label for="name">Project Name</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="name"
            v-model="formData.name"
            type="text"
            required
            placeholder="My Awesome Project"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="githubOwner">GitHub Owner</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="githubOwner"
            v-model="formData.githubOwner"
            type="text"
            required
            placeholder="username or organization"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="githubRepo">GitHub Repository</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="githubRepo"
            v-model="formData.githubRepo"
            type="text"
            required
            placeholder="repository-name"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="port">Port</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="port"
            v-model.number="formData.port"
            type="number"
            required
            min="1"
            max="65535"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="desiredRevisionSHA">Initial Revision SHA (optional)</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-1.5 text-sm"
            id="desiredRevisionSHA"
            v-model="formData.desiredRevisionSHA"
            type="text"
            placeholder="Leave empty for latest"
          />
        </div>

        <div v-if="error">
          <p style="color: red">{{ error }}</p>
        </div>

        <div>
          <DButton type="submit" :disabled="isSubmitting" variant="primary" size="lg">
            {{ isSubmitting ? "Creating..." : "Create Project" }}
          </DButton>
        </div>
      </form>
    </div>
  </DPageWrapper>
</template>
