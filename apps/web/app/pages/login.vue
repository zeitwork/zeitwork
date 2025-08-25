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
  <div>
    <div class="p-8">
      <h1 class="text-neutral mb-4 text-4xl font-bold">Welcome to Zeitwork</h1>
      <p class="text-neutral-subtle mb-4 max-w-md text-sm">If your app runs in Docker, Zeitwork can run it.</p>
      <DButton :loading variant="primary" @click="login">Login with GitHub</DButton>
    </div>
  </div>
</template>
