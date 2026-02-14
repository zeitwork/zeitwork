<script setup lang="ts">
definePageMeta({ layout: "auth" });
</script>

<template>
  <div class="bg-surface-0 min-h-screen">
    <div class="border-edge border-b">
      <div class="mx-auto max-w-5xl px-8 py-8">
        <p class="text-tertiary text-copy-sm mb-1 font-mono">experiment</p>
        <h1 class="text-primary text-title-md mb-2">Ground + Relative State</h1>
        <p class="text-secondary text-copy-lg max-w-2xl">
          Testing whether state colors can be derived from an inherited
          <code class="text-primary bg-surface-1 rounded px-1 font-mono text-xs">--ground</code>
          variable via CSS cascade, so the same state class looks correct on any surface.
        </p>
      </div>
    </div>

    <div class="mx-auto max-w-5xl space-y-12 px-8 py-12">

      <!--
        TEST 1: Same state buttons on different grounds
        This is the core test — does bg-state-1 look equally visible on all 3 grounds?
      -->
      <section>
        <h2 class="text-primary text-title-sm mb-2">Test 1: State visibility across grounds</h2>
        <p class="text-secondary text-copy mb-6">
          Each row is a different ground level. The state boxes inside should have
          <strong>equal perceived contrast</strong> against their ground, because
          <code class="text-primary font-mono text-xs">bg-state-*</code> derives from
          the inherited <code class="text-primary font-mono text-xs">--ground</code>.
        </p>

        <div class="space-y-4">
          <div
            v-for="g in [0, 1, 2]"
            :key="g"
            :class="`ground-${g}`"
            class="rounded-xl p-6"
          >
            <p class="text-primary text-copy-sm mb-3 font-mono">
              ground-{{ g }}
              <span class="text-tertiary ml-2">--ground is set here, children inherit it</span>
            </p>
            <div class="flex items-center gap-3">
              <div class="bg-state-1 rounded-lg px-4 py-3">
                <p class="text-primary text-copy-sm font-mono">state-1</p>
                <p class="text-tertiary text-copy-xs">hover</p>
              </div>
              <div class="bg-state-2 rounded-lg px-4 py-3">
                <p class="text-primary text-copy-sm font-mono">state-2</p>
                <p class="text-tertiary text-copy-xs">active</p>
              </div>
              <div class="bg-state-3 rounded-lg px-4 py-3">
                <p class="text-primary text-copy-sm font-mono">state-3</p>
                <p class="text-tertiary text-copy-xs">pressed</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        TEST 2: The original problem — active tab on nav vs on card
        Simulates the Figma scenario: same active highlight on dark nav and bright card
      -->
      <section>
        <h2 class="text-primary text-title-sm mb-2">Test 2: Active tab on different grounds</h2>
        <p class="text-secondary text-copy mb-6">
          The active tab highlight uses <code class="text-primary font-mono text-xs">bg-state-2</code>.
          It should be equally visible on the dark nav (ground-0) and the bright card (ground-2).
        </p>

        <div class="space-y-6">
          <!-- Nav simulation on ground-0 -->
          <div>
            <p class="text-tertiary text-copy-xs mb-2 font-mono">navbar on ground-0</p>
            <div class="ground-0 flex items-center gap-1 rounded-xl p-3">
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Overview</div>
              <div class="bg-state-2 text-primary text-copy-sm rounded-md px-3 py-1.5 font-medium">Deployments</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Logs</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Settings</div>
            </div>
          </div>

          <!-- Nav simulation on ground-1 -->
          <div>
            <p class="text-tertiary text-copy-xs mb-2 font-mono">navbar on ground-1</p>
            <div class="ground-1 flex items-center gap-1 rounded-xl p-3">
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Overview</div>
              <div class="bg-state-2 text-primary text-copy-sm rounded-md px-3 py-1.5 font-medium">Deployments</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Logs</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Settings</div>
            </div>
          </div>

          <!-- Nav simulation on ground-2 -->
          <div>
            <p class="text-tertiary text-copy-xs mb-2 font-mono">navbar on ground-2</p>
            <div class="ground-2 flex items-center gap-1 rounded-xl p-3">
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Overview</div>
              <div class="bg-state-2 text-primary text-copy-sm rounded-md px-3 py-1.5 font-medium">Deployments</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Logs</div>
              <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Settings</div>
            </div>
          </div>
        </div>
      </section>

      <!--
        TEST 3: Nested grounds — does --ground re-scope correctly?
        A card (ground-2) sitting on a backdrop (ground-0), with states inside the card
      -->
      <section>
        <h2 class="text-primary text-title-sm mb-2">Test 3: Nested grounds</h2>
        <p class="text-secondary text-copy mb-6">
          A ground-2 card inside a ground-0 backdrop. States inside the card
          should derive from ground-2, not ground-0.
        </p>

        <div class="ground-0 rounded-xl p-6">
          <p class="text-tertiary text-copy-xs mb-3 font-mono">ground-0 (backdrop)</p>

          <div class="ground-2 border-edge rounded-lg border p-5 shadow-sm">
            <p class="text-tertiary text-copy-xs mb-3 font-mono">ground-2 (card) — --ground re-scoped here</p>

            <div class="flex items-center gap-3">
              <div class="bg-state-1 rounded-md px-3 py-2">
                <p class="text-primary text-copy-sm font-mono">state-1</p>
              </div>
              <div class="bg-state-2 rounded-md px-3 py-2">
                <p class="text-primary text-copy-sm font-mono">state-2</p>
              </div>
              <div class="bg-state-3 rounded-md px-3 py-2">
                <p class="text-primary text-copy-sm font-mono">state-3</p>
              </div>
            </div>
          </div>

          <div class="mt-4 flex items-center gap-3">
            <p class="text-tertiary text-copy-xs font-mono">states directly on ground-0:</p>
            <div class="bg-state-1 rounded-md px-3 py-2">
              <p class="text-primary text-copy-sm font-mono">state-1</p>
            </div>
            <div class="bg-state-2 rounded-md px-3 py-2">
              <p class="text-primary text-copy-sm font-mono">state-2</p>
            </div>
          </div>
        </div>
      </section>

      <!--
        TEST 4: Full layout simulation
        Combining grounds and states like the real app
      -->
      <section class="pb-16">
        <h2 class="text-primary text-title-sm mb-2">Test 4: Full layout</h2>
        <p class="text-secondary text-copy mb-6">
          Simulated app layout using ground levels for spatial hierarchy and
          state levels for interaction.
        </p>

        <div class="ground-0 overflow-hidden rounded-xl">
          <!-- Top nav on ground-0 -->
          <div class="border-edge flex items-center gap-1 border-b px-4 py-2">
            <div class="text-primary text-copy-sm mr-4 font-medium">zeitwork</div>
            <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Overview</div>
            <div class="bg-state-2 text-primary text-copy-sm rounded-md px-3 py-1.5 font-medium">Deployments</div>
            <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Settings</div>
          </div>

          <!-- Content area steps up to ground-1 -->
          <div class="ground-1 p-6">
            <p class="text-primary text-label mb-4">Recent deployments</p>

            <!-- Card steps up to ground-2 -->
            <div class="ground-2 border-edge divide-edge divide-y overflow-hidden rounded-lg border shadow-sm">
              <div class="flex items-center justify-between px-4 py-3 hover:bg-state-1 transition-colors">
                <div>
                  <p class="text-primary text-copy-sm">my-app</p>
                  <p class="text-tertiary text-copy-xs">deployed 2h ago</p>
                </div>
                <div class="bg-state-1 text-copy-xs text-secondary rounded-full px-2 py-0.5">production</div>
              </div>
              <div class="flex items-center justify-between px-4 py-3 hover:bg-state-1 transition-colors">
                <div>
                  <p class="text-primary text-copy-sm">api-service</p>
                  <p class="text-tertiary text-copy-xs">deployed 1d ago</p>
                </div>
                <div class="bg-state-1 text-copy-xs text-secondary rounded-full px-2 py-0.5">staging</div>
              </div>
              <div class="flex items-center justify-between px-4 py-3 hover:bg-state-1 transition-colors">
                <div>
                  <p class="text-primary text-copy-sm">docs-site</p>
                  <p class="text-tertiary text-copy-xs">deployed 3d ago</p>
                </div>
                <div class="bg-state-1 text-copy-xs text-secondary rounded-full px-2 py-0.5">preview</div>
              </div>
            </div>
          </div>
        </div>
      </section>

    </div>
  </div>
</template>
