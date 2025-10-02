<script setup lang="ts">
definePageMeta({
  layout: "auth",
})

const route = useRoute()
const loading = ref(false)
const errorMessage = ref<string | null>(null)

// Check for error in query params
onMounted(() => {
  if (route.query.error) {
    errorMessage.value = String(route.query.error)
  }
})

async function login() {
  loading.value = true
  errorMessage.value = null
  await navigateTo("/auth/github", { external: true })
}

function dismissError() {
  errorMessage.value = null
  // Clean up the URL
  navigateTo("/login", { replace: true })
}
</script>

<template>
  <div class="flex flex-1 justify-center pt-[15vh]">
    <div class="flex flex-col items-center p-8 text-center">
      <div class="mb-4">
        <DLogo class="h-5 text-black" />
      </div>
      <h1 class="text-neutral text-md mb-2 font-medium">Welcome to Zeitwork</h1>
      <p class="text-neutral-subtle mb-4 max-w-md text-sm">Log in or sign up to get started.</p>

      <!-- Error message display -->
      <div v-if="errorMessage" class="mb-4 w-full max-w-md rounded-lg border border-red-200 bg-red-50 p-3 text-left">
        <div class="flex items-start justify-between">
          <div class="flex items-start">
            <svg class="mt-0.5 mr-2 h-5 w-5 flex-shrink-0 text-red-600" fill="currentColor" viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                clip-rule="evenodd"
              />
            </svg>
            <div class="flex-1">
              <h3 class="text-sm font-medium text-red-800">Authentication Error</h3>
              <p class="mt-1 text-sm text-red-700">{{ errorMessage }}</p>
              <p class="mt-1 text-xs text-red-600">Please try again. If the problem persists, contact support.</p>
            </div>
          </div>
          <button @click="dismissError" class="ml-2 text-red-500 hover:text-red-700" aria-label="Dismiss error">
            <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                clip-rule="evenodd"
              />
            </svg>
          </button>
        </div>
      </div>

      <DButton :loading variant="primary" @click="login">Continue with GitHub</DButton>
    </div>
  </div>
</template>
