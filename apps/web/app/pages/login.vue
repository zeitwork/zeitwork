<script setup lang="ts">
definePageMeta({
  layout: "auth",
})

const githubLoginUrl = computed(() => {
  return `https://github.com/login/oauth/authorize?client_id=${useRuntimeConfig().public.githubClientId}`
})
</script>

<template>
  <div>
    <div class="p-8">
      <h1 class="text-neutral mb-4 text-4xl font-bold">Zeitwork</h1>
      <p class="text-neutral-subtle mb-4 max-w-md text-sm">
        Zeitwork is a platform for managing your projects. It allows you to create projects, add domains, and manage
        storage.
      </p>
      <DButton variant="primary" :to="githubLoginUrl" :external="true">Login with GitHub</DButton>
    </div>
  </div>
</template>
