<script setup lang="ts">
import { XMarkIcon } from "@heroicons/vue/16/solid"

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
        <DLogo class="text-neutral-strong h-5" />
      </div>
      <h1 class="text-neutral text-md mb-2 font-medium">Welcome to Zeitwork</h1>
      <p class="text-neutral-subtle mb-4 max-w-md text-sm">Log in or sign up to get started.</p>

      <!-- Error message display -->
      <div
        v-if="errorMessage"
        class="border-danger bg-danger-subtle mb-4 w-full max-w-md rounded-lg border p-3 text-left"
      >
        <div class="flex items-center justify-between gap-4">
          <div class="flex items-start">
            <svg class="text-danger mt-0.5 mr-2 h-5 w-5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                clip-rule="evenodd"
              />
            </svg>
            <div class="flex-1">
              <h3 class="text-danger text-sm font-medium">Authentication Error</h3>
              <p class="text-danger mt-1 text-sm">{{ errorMessage }}</p>
              <p class="text-danger mt-1 text-xs">Please try again. If the problem persists, contact support.</p>
            </div>
          </div>
          <DButton @click="dismissError" variant="danger-light" :icon-left="XMarkIcon" />
        </div>
      </div>

      <DButton :loading variant="primary" @click="login">Continue with GitHub</DButton>
    </div>
  </div>
</template>
