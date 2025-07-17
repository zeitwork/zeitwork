<script setup lang="ts">
definePageMeta({
  layout: "marketing",
})

const email = ref("")
const isSuccess = ref<boolean | null>(null)
const responseMessage = ref("")

async function handleSubmit() {
  console.log("handleSubmit", email.value)
  if (!email.value) return

  try {
    const res = await $fetch("/api/waitlist", {
      method: "POST",
      body: {
        email: email.value,
      },
    })
    if (res.success) {
      email.value = ""
      responseMessage.value = "You're on the waitlist!"
      isSuccess.value = true
    }
  } catch (error) {
    console.error(error)
    responseMessage.value = "Something went wrong. Please try again."
    isSuccess.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-black">
    <MarketingNavigation />
    <DWrapper>
      <div class="flex flex-col gap-12 py-15 md:py-42">
        <div class="flex flex-col items-start gap-3">
          <div
            class="text-copy-sm mb-2.5 inline-flex rounded-full bg-green-900 px-2.5 py-1 font-bold text-green-400 uppercase"
          >
            Coming Soon
          </div>

          <h1 class="font-display max-w-3xl text-4xl text-white md:text-6xl">
            The fastest way to deploy and scale any application
          </h1>
          <p class="max-w-xl text-neutral-400">
            Connect your repository, and every commit triggers a new deployment. If your app has a Dockerfile, Zeitwork
            can run it.
          </p>
        </div>
        <div>
          <form @submit.prevent="handleSubmit" class="flex flex-col gap-2">
            <div class="flex gap-2">
              <input
                id="email"
                v-model="email"
                autocomplete="email"
                type="email"
                placeholder="Email"
                class="inline-flex h-9 cursor-text items-center rounded-xl border border-transparent bg-neutral-700 px-3 text-sm text-neutral-100 transition-all duration-100 select-none hover:bg-neutral-600 focus:ring-2 focus:ring-neutral-500 focus:outline-none active:bg-neutral-500"
              />
              <MarketingButton type="submit" variant="primary">Join waitlist</MarketingButton>
            </div>
            <div v-if="responseMessage" class="text-copy-sm" :class="[isSuccess ? 'text-green-400' : 'text-red-400']">
              {{ responseMessage }}
            </div>
          </form>
        </div>
      </div>
    </DWrapper>

    <div class="relative overflow-hidden">
      <DWrapper class="relative z-10">
        <div class="min-w-[900px] overflow-hidden rounded-t-xl border-x border-t border-white/30 bg-white/10 px-2 pt-2">
          <img class="w-full rounded-t-md" src="/deployments.png" alt="" />
        </div>
      </DWrapper>
    </div>
  </div>
</template>
