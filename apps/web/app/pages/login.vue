<script setup lang="ts">
definePageMeta({
  layout: "auth",
})

const loading = ref(false)

async function login() {
  loading.value = true
  await navigateTo("/auth/github", { external: true })
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
      <DButton :loading variant="primary" @click="login">Continue with GitHub</DButton>
    </div>
  </div>
</template>
