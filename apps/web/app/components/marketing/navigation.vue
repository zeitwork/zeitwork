<template>
  <DWrapper>
    <div class="flex items-center justify-between px-4 py-8 md:px-8">
      <DLogo class="h-5 text-white md:h-6" full />
      <div class="ml-4 flex gap-1 md:ml-8">
        <MarketingButton variant="secondary" to="https://github.com/zeitwork/zeitwork" target="_blank">
          <Icon name="uil:github" size="1.5em" class="md:mr-2" />
          <span class="hidden md:inline">GitHub</span>
        </MarketingButton>
        <MarketingButton variant="secondary" to="https://discord.gg/GBgRbjMDpc" target="_blank">
          <Icon name="ic:baseline-discord" size="1.5em" class="md:mr-2" />
          <span class="hidden md:inline">Discord</span>
        </MarketingButton>
        <MarketingButton variant="secondary" to="https://x.com/zeitwork" target="_blank">
          <Icon name="ri:twitter-x-fill" size="1.5em" class="md:mr-2" />
        </MarketingButton>
      </div>
    </div>
  </DWrapper>
</template>
