<script setup lang="ts">
import { useWindowScroll } from "@vueuse/core"

definePageMeta({
  layout: "marketing",
})

useHead({
  script: [
    ...(process.env.NODE_ENV === "production"
      ? [
          {
            defer: true,
            "data-domain": "zeitwork.com",
            src: "https://plausible.io/js/script.js",
          },
        ]
      : []),
  ],
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

const { x, y } = useWindowScroll()

const isAtTop = computed(() => y.value < 50)
</script>

<template>
  <div class="min-h-screen bg-white">
    <header
      class="navbar fixed top-2 left-1/2 z-50 mx-auto flex w-[calc(100%-2rem)] max-w-[78rem] -translate-x-1/2 justify-between rounded-2xl bg-white p-2 transition-all duration-300"
      :class="isAtTop ? 'border border-transparent' : 'border border-neutral-200 shadow'"
    >
      <div class="flex items-center gap-4 px-2">
        <NuxtLink to="/">
          <Wordmark />
        </NuxtLink>
        <div
          class="hidden rounded-xl border border-green-600/10 bg-green-100 px-2.5 py-1 text-sm font-medium text-green-600 md:inline"
        >
          coming soon
        </div>
      </div>
      <div class="flex items-center gap-2">
        <NuxtLink
          to="https://discord.gg/GBgRbjMDpc"
          target="_blank"
          external
          class="flex items-center justify-center rounded-lg px-2.5 py-2 hover:bg-neutral-100"
        >
          <Icon name="ri:discord-fill" size="1.5em" />
        </NuxtLink>
        <NuxtLink
          to="https://x.com/zeitwork"
          target="_blank"
          external
          class="hidden items-center justify-center rounded-lg px-2.5 py-2 hover:bg-neutral-100 md:flex"
        >
          <Icon name="ri:twitter-x-fill" size="1.5em" />
        </NuxtLink>
        <NuxtLink
          to="https://github.com/zeitwork/zeitwork"
          target="_blank"
          external
          class="flex items-center justify-center rounded-lg px-2.5 py-2 hover:bg-neutral-100"
        >
          <Icon name="uil:github" size="1.5em" />
        </NuxtLink>
        <!-- <NuxtLink
          class="inline rounded-xl bg-neutral-100 px-4 py-2.5 text-sm text-neutral-900 outline-offset-1 hover:bg-neutral-200 focus:outline-2 active:bg-neutral-200"
          to="/login"
        >
          Sign In
        </NuxtLink>
        <NuxtLink
          class="inline rounded-xl bg-neutral-900 px-4 py-2.5 text-sm text-white outline-offset-1 hover:bg-neutral-800 focus:outline-2 active:bg-neutral-700"
          to="/login"
        >
          Sign Up
        </NuxtLink> -->
      </div>
    </header>
    <div class="pt-24"></div>
    <section class="px-4 py-20">
      <div class="mx-auto mb-8 text-center"></div>
      <h1 class="mx-auto mb-8 max-w-4xl text-center text-4xl md:text-6xl">
        The fastest way to deploy <br class="hidden md:block" />
        and scale <span class="font-bold">any</span> application
      </h1>
      <p class="mx-auto mb-10 max-w-xl text-center text-lg text-neutral-500">
        Deploy anything with the simplicity of serverless. Connect your repo, push your code, and watch your app scale
        automatically.
      </p>
      <div class="flex justify-center gap-3">
        <form @submit.prevent="handleSubmit" class="flex w-full max-w-md flex-col justify-center gap-2">
          <div class="flex w-full flex-col justify-center gap-2 md:flex-row">
            <input
              id="email"
              v-model="email"
              autocomplete="email"
              type="email"
              placeholder="Email"
              class="inline rounded-xl bg-neutral-100 px-4 py-2.5 text-sm text-neutral-900 outline-offset-0 hover:bg-neutral-200 focus:outline-2 active:bg-neutral-200"
            />
            <button
              type="submit"
              class="inline rounded-xl bg-neutral-900 px-4 py-2.5 text-sm text-white outline-offset-1 hover:bg-neutral-800 focus:outline-2 active:bg-neutral-700"
            >
              Join Waitlist
            </button>
          </div>
          <div
            v-if="responseMessage"
            class="text-copy-sm mx-auto mt-4 text-center font-medium"
            :class="[isSuccess ? 'text-green-600' : 'text-red-600']"
          >
            {{ responseMessage }}
          </div>
        </form>
        <!-- <NuxtLink
          class="inline rounded-xl bg-neutral-900 px-4 py-2.5 text-sm text-white outline-offset-1 hover:bg-neutral-800 focus:outline-2 active:bg-neutral-700"
          to="/login"
        >
          Start Building
        </NuxtLink>
        <NuxtLink
          class="inline rounded-xl bg-neutral-100 px-4 py-2.5 text-sm text-neutral-900 outline-offset-1 hover:bg-neutral-200 focus:outline-2 active:bg-neutral-200"
          to="/docs"
        >
          View Docs
        </NuxtLink> -->
      </div>
    </section>
    <section class="border-b border-neutral-200 pt-20">
      <div class="relative">
        <img src="/deployments-v2.png" width="2400" height="1256" alt="" class="mx-auto w-full max-w-7xl px-4" />
        <div
          class="absolute bottom-0 left-1/2 h-40 w-full -translate-x-1/2 bg-linear-to-b from-transparent to-neutral-950/10"
        ></div>
      </div>
    </section>
    <section class="border-b border-neutral-200 px-4">
      <div class="mx-auto max-w-7xl py-20">
        <h2 class="mb-8 text-3xl text-neutral-900 md:text-5xl">Built for applications that don't fit in functions</h2>
        <p class="mb-4 text-neutral-800"><strong class="font-semibold">Your app doesn't fit in a Lambda?</strong></p>
        <p class="max-w-xl text-neutral-500">
          Whether it's a Rails monolith, a machine learning model, a game backend, or a full-stack Next.js app.
          Serverless platforms force you into their constraints. We don't. If it can run, it runs on Zeitwork. Full
          stop.
        </p>
      </div>
    </section>
    <section class="border-b border-neutral-200 px-4">
      <div class="mx-auto max-w-7xl py-20">
        <h2 class="mb-8 text-3xl text-neutral-900 md:text-5xl">Developer experience without the limitations</h2>
        <p class="mb-8 max-w-xl text-neutral-500">No infrastructure expertise required.</p>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
          <div class="rounded-md border border-neutral-200">
            <img src="/images/deploy-from-git.png" alt="" class="w-full p-6 pr-0" />
            <div class="p-6">
              <h3 class="mb-2 font-medium text-neutral-800">Deploy from Git</h3>
              <p class="text-neutral-500">
                Every commit triggers a new deployment. Branch previews, rollbacks, and deployment history included.
              </p>
            </div>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <img src="/images/zero-configuration.png" alt="" class="mx-auto p-6" />
            <h3 class="mb-2 font-medium text-neutral-800">Zero configuration</h3>
            <p class="text-neutral-500">
              No Dockerfile? No problem. We detect your framework and build it automatically. Bring your own Dockerfile
              for full control.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <img src="/images/auto-scaling.png" alt="" class="mx-auto p-6" />
            <h3 class="mb-2 font-medium text-neutral-800">Auto-scaling that actually works</h3>
            <p class="text-neutral-500">
              Handle traffic spikes without manual intervention. Scale to zero when idle, scale to thousands when
              needed.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">80% cheaper than AWS</h3>
            <p class="text-neutral-500">
              We optimize infrastructure costs behind the scenes. You get enterprise performance at startup prices.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">No DevOps required</h3>
            <p class="text-neutral-500">
              We handle all infrastructure, security patches, SSL certificates, and scaling. You just push code.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Full application support</h3>
            <p class="text-neutral-500">
              Long-running processes, background jobs, WebSockets, scheduled tasks—everything your app needs to run in
              production.
            </p>
          </div>
        </div>
      </div>
    </section>
    <!--  -->
    <section class="border-b border-neutral-200 px-4">
      <div class="mx-auto max-w-7xl py-20">
        <h2 class="mb-8 text-3xl text-neutral-900 md:text-5xl">How it works</h2>
        <p class="mb-8 max-w-xl text-neutral-500">It's so simple that even your grandma could use it!</p>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Connect your repository</h3>
            <p class="text-neutral-500">
              Link your GitHub, GitLab, or Bitbucket repo. We'll detect your framework automatically.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Push your code</h3>
            <p class="text-neutral-500">
              Every push to main deploys to production. Create branches for preview environments.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Scale automatically</h3>
            <p class="text-neutral-500">
              We monitor your app and scale resources based on real usage. Pay only for what you use.
            </p>
          </div>
        </div>
      </div>
    </section>
    <!--  -->
    <section class="border-b border-neutral-200 px-4">
      <div class="mx-auto max-w-7xl py-20">
        <h2 class="mb-8 text-3xl text-neutral-900 md:text-5xl">Run anything, anywhere</h2>
        <p class="mb-8 max-w-xl text-neutral-500">We're serious! In fact, we will help you optimize it.</p>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Web Applications</h3>
            <p class="text-neutral-500">
              Full-stack frameworks, APIs, microservices. If it serves HTTP, we'll scale it.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Background Jobs</h3>
            <p class="text-neutral-500">Long-running processes, queue workers, scheduled tasks. They just work.</p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Data & ML</h3>
            <p class="text-neutral-500">
              Jupyter notebooks, training pipelines, inference servers. GPU support coming soon.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Games & Real-time</h3>
            <p class="text-neutral-500">
              WebSocket servers, game backends, collaborative tools. Persistent connections included.
            </p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Internal Tools</h3>
            <p class="text-neutral-500">Admin panels, dashboards, development environments. Secure by default.</p>
          </div>
          <div class="rounded-md border border-neutral-200 p-6">
            <h3 class="mb-2 font-medium text-neutral-800">Legacy Systems</h3>
            <p class="text-neutral-500">
              That old Java app? The PHP monolith? If you can containerize it, we can run it.
            </p>
          </div>
        </div>
      </div>
    </section>
    <!--  -->
    <section class="border-b border-neutral-200 px-4">
      <div class="mx-auto max-w-7xl py-20">
        <h2 class="mb-8 text-center text-3xl text-neutral-900 md:text-5xl">Ready to ship faster?</h2>
        <p class="mx-auto mb-8 max-w-xs text-center text-neutral-500">
          Stop wrestling with infrastructure. Stop overpaying for cloud. Start building.
        </p>
        <div class="flex justify-center gap-3">
          <form @submit.prevent="handleSubmit" class="flex w-full max-w-md flex-col justify-center gap-2">
            <div class="flex w-full flex-col justify-center gap-2 md:flex-row">
              <input
                id="email"
                v-model="email"
                autocomplete="email"
                type="email"
                placeholder="Email"
                class="inline rounded-xl bg-neutral-100 px-4 py-2.5 text-sm text-neutral-900 outline-offset-0 hover:bg-neutral-200 focus:outline-2 active:bg-neutral-200"
              />
              <button
                type="submit"
                class="inline rounded-xl bg-neutral-900 px-4 py-2.5 text-sm text-white outline-offset-1 hover:bg-neutral-800 focus:outline-2 active:bg-neutral-700"
              >
                Join Waitlist
              </button>
            </div>
            <div
              v-if="responseMessage"
              class="text-copy-sm mx-auto mt-4 text-center font-medium"
              :class="[isSuccess ? 'text-green-600' : 'text-red-600']"
            >
              {{ responseMessage }}
            </div>
          </form>
        </div>
      </div>
    </section>
  </div>
</template>
