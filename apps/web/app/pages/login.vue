<script setup lang="ts">
definePageMeta({
  layout: "auth",
})

const loading = ref(false)

async function loginGithub() {
  loading.value = true
  await navigateTo("/auth/github", { external: true })
}

async function loginMyspace() {
  loading.value = true
  await navigateTo("/auth/myspace", { external: true })
}
</script>

<template>
  <div>
    <div class="p-8">
      <h1 class="text-neutral mb-4 text-4xl font-bold">Welcome to Zeitwork</h1>
      <p class="text-neutral-subtle mb-4 max-w-md text-sm">If your app runs in Docker, Zeitwork can run it.</p>
      <div class="flex flex-col w-max">
        <DButton :loading variant="primary" @click="loginGithub">
          <span class="flex items-center">
            <img src="/icons/github-mark.svg" alt="GitHub Logo" class="h-5 w-5 mr-2" />
            Login with GitHub
          </span>
        </DButton>
        <DButton :loading variant="primary" @click="loginMyspace" class="mt-2">
          <span class="flex items-center">
            <img src="/icons/myspace.svg" alt="MySpace Logo" class="h-5 w-5 mr-2" />
            Login with MySpace
          </span>
        </DButton>
      </div>
    </div>
  </div>
</template>