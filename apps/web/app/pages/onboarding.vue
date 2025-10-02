<script setup lang="ts">
definePageMeta({
  layout: "auth",
})

const config = useRuntimeConfig()
const loading = ref(false)
const checkoutLoading = ref<string | null>(null)
const { $posthog } = useNuxtApp()

// Track onboarding page view
onMounted(() => {
  $posthog().capture("onboarding_viewed")
})

async function subscribe(plan: "hobby" | "business") {
  checkoutLoading.value = plan

  // Track plan selection
  $posthog().capture("plan_selected", {
    plan: plan,
    price: plan === "hobby" ? "$5/month" : "$25/month",
  })

  try {
    const priceId = plan === "hobby" ? config.public.stripe.planHobbyId : config.public.stripe.planBusinessId

    if (!priceId) {
      console.error(`Price ID not configured for ${plan} plan`)
      return
    }

    const response = await $fetch("/api/checkout", {
      method: "POST",
      body: {
        priceId,
      },
    })

    if (response?.url) {
      // Track checkout initiated
      $posthog().capture("checkout_initiated", {
        plan: plan,
        price_id: priceId,
      })

      // Redirect to Stripe checkout
      window.location.href = response.url
    }
  } catch (err) {
    console.error("Failed to create checkout session:", err)
    $posthog().capture("checkout_failed", {
      plan: plan,
      error: err instanceof Error ? err.message : "Unknown error",
    })
  } finally {
    checkoutLoading.value = null
  }
}
</script>

<template>
  <div>
    <div class="mx-auto flex max-w-4xl flex-1 flex-col p-4 pt-24">
      <h1 class="text-neutral mb-2 text-2xl">Pricing</h1>
      <p class="text-neutral-subtle mb-8 text-sm">
        During the beta period, we require users to have a valid credit card on file.
      </p>
      <h2 class="text-neutral mb-2 text-lg">Pick a plan</h2>
      <div class="grid grid-cols-3 gap-4">
        <div class="bg-neutral-subtle border-neutral-subtle flex flex-col gap-2 rounded-sm border p-2">
          <h3 class="text-neutral">Hobby</h3>
          <p class="text-neutral-subtle">$5/month</p>
          <ul role="list" class="mt-9/12 space-y-2/12 text-neutral text-sm">
            <li class="flex gap-x-1.5"><span>✓</span> One region</li>
            <li class="flex gap-x-1.5"><span>✓</span> Free Zeitwork domain</li>
            <li class="flex gap-x-1.5"><span>✓</span> DDoS protection</li>
            <li class="flex gap-x-1.5"><span>✓</span> Custom Dockerfile</li>
          </ul>
          <DButton variant="primary" @click="subscribe('hobby')" :disabled="!!checkoutLoading">
            {{ checkoutLoading === "hobby" ? "Loading..." : "Get Hobby" }}
          </DButton>
        </div>
        <div class="bg-neutral-subtle border-neutral-subtle flex flex-col gap-2 rounded-sm border p-2">
          <h3 class="text-neutral">Business</h3>
          <p class="text-neutral-subtle">$25/month</p>
          <ul role="list" class="mt-9/12 space-y-2/12 text-neutral text-sm">
            <li class="flex gap-x-1.5"><span>✓</span> Multiple regions</li>
            <li class="flex gap-x-1.5"><span>✓</span> Free Zeitwork domain</li>
            <li class="flex gap-x-1.5"><span>✓</span> DDoS protection</li>
            <li class="flex gap-x-1.5"><span>✓</span> Custom Dockerfile</li>
          </ul>
          <DButton variant="primary" @click="subscribe('business')" :disabled="!!checkoutLoading">
            {{ checkoutLoading === "business" ? "Loading..." : "Get Business" }}
          </DButton>
        </div>
        <div class="bg-neutral-subtle border-neutral-subtle flex flex-col gap-2 rounded-sm border p-2">
          <h3 class="text-neutral">Enterprise</h3>
          <p class="text-neutral-subtle">Talk to us</p>
          <ul role="list" class="mt-9/12 space-y-2/12 text-neutral text-sm">
            <li class="flex gap-x-1.5"><span>✓</span> All regions</li>
            <li class="flex gap-x-1.5"><span>✓</span> Free Zeitwork domain</li>
            <li class="flex gap-x-1.5"><span>✓</span> DDoS protection</li>
            <li class="flex gap-x-1.5"><span>✓</span> Custom Dockerfile</li>
          </ul>
          <DButton variant="primary" to="mailto:sales@zeitwork.com">Talk to us</DButton>
        </div>
      </div>
      <p class="text-neutral-subtle mt-8 text-sm">
        Our free plan will be available to everyone after our public launch. <br />
        You can request a refund any time by emailing
        <a href="mailto:support@zeitwork.com" class="text-neutral">support@zeitwork.com</a>.
      </p>
    </div>
  </div>
</template>
