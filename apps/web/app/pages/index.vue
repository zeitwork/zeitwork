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
          <h1 class="font-display max-w-3xl text-6xl text-white">
            The fastest way to deploy and scale any application
          </h1>
          <p class="text-neutral-200">The open-source alternative that turns intentions into actions.</p>
        </div>
        <div class="flex gap-2">
          <MarketingButton variant="primary"> Get Started </MarketingButton>
          <MarketingButton>Watch demo</MarketingButton>
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
