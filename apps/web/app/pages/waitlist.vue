<script setup lang="ts">
definePageMeta({
  layout: "auth",
});

const { clear } = useUserSession();

async function logout() {
  await clear();
  await navigateTo("/login");
}
</script>

<template>
  <div class="flex flex-1 justify-center pt-[15vh]">
    <div class="flex flex-col items-center p-8 text-center">
      <div class="mb-4">
        <DLogo class="text-neutral-strong h-5" />
      </div>

      <h1 class="text-neutral text-md mb-2 font-medium">Welcome to Zeitwork</h1>

      <p class="text-neutral-subtle mb-8 max-w-md text-sm">
        We're letting people in slowly to test our system. If you want early access, DM Tom on X or
        join our Discord server.
      </p>

      <div class="flex flex-col gap-3">
        <a href="https://x.com/tomhaerter" target="_blank" rel="noopener noreferrer">
          <DButton variant="secondary" class="w-full">DM @tomhaerter on X</DButton>
        </a>

        <a href="https://discord.gg/GBgRbjMDpc" target="_blank" rel="noopener noreferrer">
          <DButton variant="secondary" class="w-full">Join Discord</DButton>
        </a>

        <DButton variant="secondary" class="w-full" @click="logout">Log out</DButton>
      </div>
    </div>
  </div>
</template>
