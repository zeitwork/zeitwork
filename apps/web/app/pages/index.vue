<script setup lang="ts">
definePageMeta({
  layout: "marketing",
})
</script>

<template>
  <div class="min-h-screen bg-black">
    <MarketingNavigation />
    <DWrapper>
      <div class="flex flex-col gap-12 py-42">
        <div class="flex flex-col gap-3">
          <div class="mb-2.5 text-sm font-bold text-green-600 uppercase">Coming Soon</div>

          <h1 class="font-display max-w-3xl text-6xl text-white">
            The fastest way to deploy and scale any application
          </h1>
          <p class="max-w-xl text-neutral-200">
            Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork
            can run it.
          </p>
        </div>
        <div class="flex gap-2">
          <MarketingButton variant="primary" to="https://github.com/zeitwork/zeitwork" target="_blank">
            Star us on GitHub
          </MarketingButton>
          <!-- <MarketingButton variant="secondary">Watch demo</MarketingButton> -->
        </div>
      </div>
    </DWrapper>

    <div class="relative overflow-hidden">
      <DWrapper class="relative z-10">
        <div class="min-w-[900px] overflow-hidden rounded-t-xl border-x border-t border-white/30 bg-white/10 px-2 pt-2">
          <img class="w-full rounded-t-md" src="/deployments.png" alt="" />
        </div>
      </DWrapper>
      <img class="absolute bottom-0 w-full" src="/hero-bg.png" />
    </div>
  </div>
</template>
