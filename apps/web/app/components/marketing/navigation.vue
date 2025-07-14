<template>
  <DWrapper>
    <div class="flex items-center justify-between py-6">
      <DLogo class="h-5 text-white md:h-6" full />
      <div class="flex gap-2">
        <!-- <MarketingButton> Sign In </MarketingButton> -->
        <!-- <MarketingButton variant="primary"> Sign Up </MarketingButton> -->
        <MarketingButton variant="secondary" to="https://github.com/zeitwork/zeitwork" target="_blank">
          GitHub
        </MarketingButton>
        <MarketingButton variant="secondary" to="https://discord.gg/GBgRbjMDpc" target="_blank">
          Discord
        </MarketingButton>
        <MarketingButton variant="secondary" to="https://x.com/zeitwork" target="_blank">X</MarketingButton>
      </div>
    </div>
  </DWrapper>
</template>
