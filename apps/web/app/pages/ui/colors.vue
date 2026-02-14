<script setup lang="ts">
import {
  SunIcon,
  SearchIcon,
  SettingsIcon,
  BellIcon,
  CircleCheckIcon,
  TriangleAlertIcon,
  CircleXIcon,
  InfoIcon,
  PlusIcon,
  BoxIcon,
} from "lucide-vue-next";

definePageMeta({ layout: "auth" });

type Swatch = {
  name: string;
  class: string;
  token: string;
  formula?: string;
};

const baseLevels: Swatch[] = [
  {
    name: "base-0",
    class: "base-0",
    token: "--color-base-0",
    formula: "l - 0.04 * contrast (from seed)",
  },
  {
    name: "base-1",
    class: "base-1",
    token: "--color-base-1",
    formula: "l - 0.01 * contrast (from seed)",
  },
  {
    name: "base-2",
    class: "base-2",
    token: "--color-base-2",
    formula: "var(--seed)",
  },
];

const surfaceLevels: Swatch[] = [
  {
    name: "surface-1",
    class: "bg-surface-1",
    token: "--color-surface-1",
    formula: "l + 0.035 * dir (from --base)",
  },
  {
    name: "surface-2",
    class: "bg-surface-2",
    token: "--color-surface-2",
    formula: "l + 0.07 * dir (from --base)",
  },
  {
    name: "surface-3",
    class: "bg-surface-3",
    token: "--color-surface-3",
    formula: "l + 0.1 * dir (from --base)",
  },
];

const textLevels: Swatch[] = [
  {
    name: "primary",
    class: "text-primary",
    token: "--text-color-primary",
    formula: "l + 0.75 * dir (from --base)",
  },
  {
    name: "secondary",
    class: "text-secondary",
    token: "--text-color-secondary",
    formula: "l + 0.55 * dir (from --base)",
  },
  {
    name: "tertiary",
    class: "text-tertiary",
    token: "--text-color-tertiary",
    formula: "l + 0.4 * dir (from --base)",
  },
];

const edgeLevels: Swatch[] = [
  {
    name: "edge-subtle",
    class: "border-edge-subtle",
    token: "--border-color-edge-subtle",
    formula: "l + 0.75 * dir / 0.06 alpha",
  },
  {
    name: "edge",
    class: "border-edge",
    token: "--border-color-edge",
    formula: "l + 0.75 * dir / 0.12 alpha",
  },
  {
    name: "edge-strong",
    class: "border-edge-strong",
    token: "--border-color-edge-strong",
    formula: "l + 0.75 * dir / 0.2 alpha",
  },
];

type IntentColor = {
  name: string;
  base: string;
  subtle: string;
  strong: string;
  on: string;
  seed: string;
};

const intents: IntentColor[] = [
  {
    name: "accent",
    base: "bg-accent",
    subtle: "bg-accent-subtle",
    strong: "bg-accent-strong",
    on: "text-accent-on",
    seed: "oklch(0.55 0.18 250)",
  },
  {
    name: "danger",
    base: "bg-danger",
    subtle: "bg-danger-subtle",
    strong: "bg-danger-strong",
    on: "text-danger-on",
    seed: "oklch(0.55 0.2 25)",
  },
  {
    name: "warn",
    base: "bg-warn",
    subtle: "bg-warn-subtle",
    strong: "bg-warn-strong",
    on: "text-warn-on",
    seed: "oklch(0.75 0.15 85)",
  },
  {
    name: "success",
    base: "bg-success",
    subtle: "bg-success-subtle",
    strong: "bg-success-strong",
    on: "text-success-on",
    seed: "oklch(0.55 0.17 155)",
  },
];
</script>

<template>
  <div class="base-2 min-h-screen">
    <!-- Header -->
    <div class="border-edge border-b">
      <div class="mx-auto max-w-6xl px-8 py-8">
        <p class="text-tertiary text-copy-sm mb-1 font-mono">app.css</p>
        <h1 class="text-primary text-title-md mb-2">OKLCH Color System</h1>
        <p class="text-secondary text-copy-lg max-w-2xl">
          Two axes: <code class="text-primary bg-surface-1 rounded px-1 font-mono text-xs">base-*</code>
          for spatial layers,
          <code class="text-primary bg-surface-1 rounded px-1 font-mono text-xs">surface-*</code>
          for interactive shifts. Everything derives from
          <code class="text-primary bg-surface-1 rounded px-1 font-mono text-xs">--seed</code>.
        </p>
      </div>
    </div>

    <div class="mx-auto max-w-6xl space-y-16 px-8 py-12">

      <!--
        SECTION 1: SEEDS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Seeds</h2>
          <p class="text-secondary text-copy">
            Two seed colors + two control values produce the entire palette.
          </p>
        </div>

        <div class="grid grid-cols-3 gap-4">
          <div class="bg-surface-1 border-edge rounded-lg border p-5">
            <div class="mb-3 flex items-center gap-2">
              <div class="base-2 border-edge size-8 rounded-full border"></div>
              <div>
                <p class="text-primary text-label">--seed</p>
                <p class="text-tertiary text-copy-sm font-mono">oklch(0.985 0.002 55)</p>
              </div>
            </div>
            <div class="space-y-1.5">
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Hue</span>
                <span class="text-secondary text-copy-sm font-mono">55</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Chroma</span>
                <span class="text-secondary text-copy-sm font-mono">0.002</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Lightness</span>
                <span class="text-secondary text-copy-sm font-mono">0.985 / 0.15</span>
              </div>
            </div>
          </div>

          <div class="bg-surface-1 border-edge rounded-lg border p-5">
            <div class="mb-3 flex items-center gap-2">
              <div class="bg-accent size-8 rounded-full"></div>
              <div>
                <p class="text-primary text-label">--accent</p>
                <p class="text-tertiary text-copy-sm font-mono">oklch(0.55 0.18 250)</p>
              </div>
            </div>
            <div class="space-y-1.5">
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Hue</span>
                <span class="text-secondary text-copy-sm font-mono">250</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Chroma</span>
                <span class="text-secondary text-copy-sm font-mono">0.18</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">Lightness</span>
                <span class="text-secondary text-copy-sm font-mono">0.55 / 0.7</span>
              </div>
            </div>
          </div>

          <div class="bg-surface-1 border-edge rounded-lg border p-5">
            <div class="mb-3 flex items-center gap-2">
              <div class="bg-surface-2 border-edge flex size-8 items-center justify-center rounded-full border">
                <SunIcon class="text-secondary size-4" />
              </div>
              <div>
                <p class="text-primary text-label">Controls</p>
                <p class="text-tertiary text-copy-sm">Direction & contrast</p>
              </div>
            </div>
            <div class="space-y-1.5">
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">--dir (light)</span>
                <span class="text-secondary text-copy-sm font-mono">-1</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">--dir (dark)</span>
                <span class="text-secondary text-copy-sm font-mono">+1</span>
              </div>
              <div class="flex justify-between">
                <span class="text-tertiary text-copy-sm">--contrast</span>
                <span class="text-secondary text-copy-sm font-mono">1</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 2: BASE LEVELS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Base Levels</h2>
          <p class="text-secondary text-copy">
            Spatial backdrop layers. Always darker-to-lighter regardless of color scheme.
            Derived from <code class="text-primary font-mono text-xs">--seed</code> (fixed, no --dir).
            Each <code class="text-primary font-mono text-xs">.base-*</code> class sets
            <code class="text-primary font-mono text-xs">--base</code> on its element &mdash;
            all children inherit it.
          </p>
        </div>

        <div class="mb-6 grid grid-cols-3 gap-4">
          <div
            v-for="b in baseLevels"
            :key="b.name"
            :class="b.class"
            class="border-edge flex aspect-[3/2] flex-col justify-between rounded-lg border p-4"
          >
            <p class="text-tertiary text-copy-xs font-mono">{{ b.formula }}</p>
            <p class="text-primary text-copy-sm font-mono">{{ b.name }}</p>
          </div>
        </div>

        <div class="bg-surface-1 border-edge rounded-lg border p-5">
          <p class="text-primary text-label mb-2">How base levels work</p>
          <p class="text-secondary text-copy mb-3">
            Base levels subtract lightness from the seed. They go in the
            <strong>same absolute direction</strong> in both light and dark mode &mdash;
            always darker. This is for spatial hierarchy: sidebar behind content, content behind card.
          </p>
          <div class="grid grid-cols-2 gap-3">
            <div class="bg-surface-2 rounded-md p-3">
              <p class="text-tertiary text-copy-sm mb-1">Light mode</p>
              <p class="text-primary font-mono text-xs">base-0 = 0.985 - 0.04 = <strong>0.945</strong></p>
              <p class="text-primary font-mono text-xs">base-1 = 0.985 - 0.01 = <strong>0.975</strong></p>
              <p class="text-primary font-mono text-xs">base-2 = seed = <strong>0.985</strong></p>
            </div>
            <div class="bg-surface-2 rounded-md p-3">
              <p class="text-tertiary text-copy-sm mb-1">Dark mode</p>
              <p class="text-primary font-mono text-xs">base-0 = 0.15 - 0.04 = <strong>0.11</strong></p>
              <p class="text-primary font-mono text-xs">base-1 = 0.15 - 0.01 = <strong>0.14</strong></p>
              <p class="text-primary font-mono text-xs">base-2 = seed = <strong>0.15</strong></p>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 3: SURFACE SHIFTS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Surface Levels</h2>
          <p class="text-secondary text-copy">
            Interactive shifts relative to the inherited
            <code class="text-primary font-mono text-xs">--base</code>.
            Uses <code class="text-primary font-mono text-xs">--dir</code> so they feel the same in both modes.
            Darker in light mode, lighter in dark mode.
          </p>
        </div>

        <div class="mb-6 grid grid-cols-3 gap-4">
          <div
            v-for="s in surfaceLevels"
            :key="s.name"
            :class="s.class"
            class="border-edge flex aspect-[3/2] flex-col justify-between rounded-lg border p-4"
          >
            <p class="text-tertiary text-copy-xs font-mono">{{ s.formula }}</p>
            <p class="text-primary text-copy-sm font-mono">{{ s.name }}</p>
          </div>
        </div>

        <div class="bg-surface-1 border-edge rounded-lg border p-5">
          <p class="text-primary text-label mb-2">Context-relative: surfaces adapt to their base</p>
          <p class="text-secondary text-copy mb-4">
            Surface tokens derive from the inherited <code class="text-primary font-mono text-xs">--base</code>
            variable. When a <code class="text-primary font-mono text-xs">.base-*</code> class overrides
            <code class="text-primary font-mono text-xs">--base</code>, all surfaces inside adapt automatically.
          </p>

          <div class="grid grid-cols-3 gap-4">
            <div
              v-for="b in baseLevels"
              :key="b.name"
              :class="b.class"
              class="rounded-lg p-4"
            >
              <p class="text-tertiary text-copy-xs mb-3 font-mono">on {{ b.name }}</p>
              <div class="flex gap-2">
                <div class="bg-surface-1 rounded-md px-3 py-2">
                  <p class="text-primary text-copy-xs font-mono">s-1</p>
                </div>
                <div class="bg-surface-2 rounded-md px-3 py-2">
                  <p class="text-primary text-copy-xs font-mono">s-2</p>
                </div>
                <div class="bg-surface-3 rounded-md px-3 py-2">
                  <p class="text-primary text-copy-xs font-mono">s-3</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 4: NESTED STACKING
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Nested Stacking</h2>
          <p class="text-secondary text-copy">
            A <code class="text-primary font-mono text-xs">base-2</code> card inside a
            <code class="text-primary font-mono text-xs">base-0</code> backdrop.
            Surface tokens inside the card derive from base-2, not base-0.
            Text and edges also adapt.
          </p>
        </div>

        <div class="base-0 rounded-xl p-6">
          <p class="text-tertiary text-copy-xs mb-4 font-mono">base-0 (backdrop) &mdash; --base is overridden here</p>

          <div class="flex gap-4">
            <div class="flex-1">
              <p class="text-tertiary text-copy-xs mb-2 font-mono">surfaces on base-0:</p>
              <div class="flex gap-2">
                <div class="bg-surface-1 rounded-md px-3 py-2">
                  <p class="text-primary text-copy-xs font-mono">surface-1</p>
                </div>
                <div class="bg-surface-2 rounded-md px-3 py-2">
                  <p class="text-primary text-copy-xs font-mono">surface-2</p>
                </div>
              </div>
            </div>
          </div>

          <div class="base-2 border-edge mt-4 rounded-lg border p-5 shadow-sm">
            <p class="text-tertiary text-copy-xs mb-4 font-mono">base-2 (card) &mdash; --base re-scoped here</p>

            <div class="flex gap-2">
              <div class="bg-surface-1 rounded-md px-3 py-2">
                <p class="text-primary text-copy-xs font-mono">surface-1</p>
              </div>
              <div class="bg-surface-2 rounded-md px-3 py-2">
                <p class="text-primary text-copy-xs font-mono">surface-2</p>
              </div>
              <div class="bg-surface-3 rounded-md px-3 py-2">
                <p class="text-primary text-copy-xs font-mono">surface-3</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 5: TEXT HIERARCHY ON BASE LEVELS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Text Hierarchy</h2>
          <p class="text-secondary text-copy">
            Text tokens derive from <code class="text-primary font-mono text-xs">--base</code>,
            so they adapt to each base level automatically.
          </p>
        </div>

        <div class="space-y-3">
          <div
            v-for="b in baseLevels"
            :key="b.name"
            :class="b.class"
            class="border-edge grid grid-cols-4 items-center gap-4 rounded-lg border px-5 py-4"
          >
            <p class="text-tertiary text-copy-xs font-mono">{{ b.name }}</p>
            <p class="text-primary text-copy">Primary</p>
            <p class="text-secondary text-copy">Secondary</p>
            <p class="text-tertiary text-copy">Tertiary</p>
          </div>
        </div>
      </section>

      <!--
        SECTION 6: EDGES ON BASE LEVELS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Edges</h2>
          <p class="text-secondary text-copy">
            Semi-transparent borders derived from
            <code class="text-primary font-mono text-xs">--base</code>.
            They adapt to each base level.
          </p>
        </div>

        <div class="grid grid-cols-3 gap-4">
          <div
            v-for="b in baseLevels"
            :key="b.name"
            :class="b.class"
            class="space-y-3 rounded-lg p-5"
          >
            <p class="text-tertiary text-copy-xs font-mono">on {{ b.name }}</p>
            <div
              v-for="e in edgeLevels"
              :key="e.name"
              class="flex items-center gap-3 rounded-md border-2 p-3"
              :class="e.class"
            >
              <span class="text-primary text-copy-sm">{{ e.name }}</span>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 7: ACTIVE TAB PROBLEM (SOLVED)
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Active Tab on Different Bases</h2>
          <p class="text-secondary text-copy">
            The key problem this system solves: an active tab using
            <code class="text-primary font-mono text-xs">bg-surface-2</code>
            should be equally visible on any base level. Because surfaces derive from the
            inherited <code class="text-primary font-mono text-xs">--base</code>, contrast is preserved.
          </p>
        </div>

        <div class="space-y-4">
          <div
            v-for="b in baseLevels"
            :key="b.name"
            :class="b.class"
            class="flex items-center gap-1 rounded-xl p-3"
          >
            <span class="text-tertiary text-copy-xs mr-3 font-mono">{{ b.name }}</span>
            <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Overview</div>
            <div class="bg-surface-2 text-primary text-copy-sm rounded-md px-3 py-1.5 font-medium">Deployments</div>
            <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Logs</div>
            <div class="text-secondary text-copy-sm rounded-md px-3 py-1.5">Settings</div>
          </div>
        </div>
      </section>

      <!--
        SECTION 8: FULL LAYOUT (moved to /ui/layout)
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Full Layout</h2>
          <p class="text-secondary text-copy">
            Faithful reproduction of the Figma layout using base-0 &rarr; base-1 &rarr; base-2 stacking.
          </p>
        </div>
        <NuxtLink
          to="/ui/layout"
          class="bg-surface-1 border-edge text-primary text-copy-sm inline-flex items-center gap-2 rounded-lg border px-4 py-3 font-medium hover:bg-surface-2 transition-colors"
        >
          Open layout test page &rarr;
        </NuxtLink>
      </section>

      <!--
        SECTION 9: DIALOG
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Dialog Anatomy</h2>
          <p class="text-secondary text-copy">
            Dialog uses surface-1 as the outer shell on the page's base level,
            then scopes a new base-2 for the content area inside.
          </p>
        </div>

        <div class="base-1 border-edge flex items-center justify-center rounded-xl border py-16">
          <div class="relative w-full max-w-[450px]">
            <div class="bg-surface-1 rounded-[14px] p-0.5 shadow-xl">
              <div class="base-2 border-edge rounded-xl border">
                <div class="border-edge border-b px-4 py-3">
                  <p class="text-primary text-label">Add Domain</p>
                </div>
                <div class="space-y-4 p-4">
                  <DFormGroup>
                    <DFormLabel>Domain name</DFormLabel>
                    <DInput placeholder="example.com" />
                  </DFormGroup>
                  <DFormGroup>
                    <DFormLabel>Branch</DFormLabel>
                    <DInput placeholder="main" leading="git:" />
                  </DFormGroup>
                </div>
              </div>
              <div class="flex items-center justify-end gap-2 p-2">
                <DButton variant="transparent" size="sm">Cancel</DButton>
                <DButton variant="primary" size="sm">Add domain</DButton>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 10: FORM ELEMENTS ON BASE LEVELS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Forms on Different Bases</h2>
          <p class="text-secondary text-copy">
            Inputs and buttons on each base level. Components don't need to know
            which base they're on &mdash; everything adapts via
            <code class="text-primary font-mono text-xs">--base</code> inheritance.
          </p>
        </div>

        <div class="grid grid-cols-3 gap-4">
          <div
            v-for="b in baseLevels"
            :key="b.name"
            :class="b.class"
            class="border-edge space-y-4 rounded-lg border p-5"
          >
            <p class="text-tertiary text-copy-xs font-mono">{{ b.name }}</p>
            <DFormGroup>
              <DFormLabel>Email</DFormLabel>
              <DInput type="email" placeholder="you@example.com" />
            </DFormGroup>
            <div class="flex gap-2">
              <DButton variant="primary" size="sm">Save</DButton>
              <DButton variant="secondary" size="sm">Cancel</DButton>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 11: INTENT COLORS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Intent Colors</h2>
          <p class="text-secondary text-copy">
            Each intent has its own OKLCH seed. These are global (not context-relative).
          </p>
        </div>

        <div class="grid grid-cols-4 gap-4">
          <div
            v-for="intent in intents"
            :key="intent.name"
            class="bg-surface-1 border-edge overflow-hidden rounded-lg border"
          >
            <div :class="intent.base" class="px-4 py-5">
              <p :class="intent.on" class="text-label">{{ intent.name }}</p>
              <p :class="intent.on" class="text-copy-xs mt-0.5 font-mono opacity-70">
                {{ intent.seed }}
              </p>
            </div>
            <div class="space-y-0 divide-y divide-edge">
              <div class="flex items-center justify-between px-4 py-3">
                <span class="text-secondary text-copy-sm">base</span>
                <div :class="intent.base" class="size-6 rounded-md"></div>
              </div>
              <div class="flex items-center justify-between px-4 py-3">
                <span class="text-secondary text-copy-sm">subtle</span>
                <div :class="intent.subtle" class="size-6 rounded-md"></div>
              </div>
              <div class="flex items-center justify-between px-4 py-3">
                <span class="text-secondary text-copy-sm">strong</span>
                <div :class="intent.strong" class="size-6 rounded-md"></div>
              </div>
              <div class="flex items-center justify-between px-4 py-3">
                <span class="text-secondary text-copy-sm">on</span>
                <div
                  :class="[intent.base, intent.on]"
                  class="flex size-6 items-center justify-center rounded-md text-xs"
                >
                  Aa
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 12: INTENT BANNERS
      -->
      <section>
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Intent Colors in Context</h2>
          <p class="text-secondary text-copy">
            Realistic alert patterns using subtle backgrounds + colored icons.
          </p>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="bg-success-subtle border-edge rounded-lg border p-4">
            <div class="flex items-start gap-3">
              <CircleCheckIcon class="text-success mt-0.5 size-5" />
              <div>
                <p class="text-primary text-label">Deployment successful</p>
                <p class="text-secondary text-copy-sm mt-1">
                  Your app is live at my-app.zeitwork.dev
                </p>
              </div>
            </div>
          </div>

          <div class="bg-danger-subtle border-edge rounded-lg border p-4">
            <div class="flex items-start gap-3">
              <CircleXIcon class="text-danger mt-0.5 size-5" />
              <div>
                <p class="text-primary text-label">Build failed</p>
                <p class="text-secondary text-copy-sm mt-1">
                  Exit code 1 &mdash; check your build logs.
                </p>
              </div>
            </div>
          </div>

          <div class="bg-warn-subtle border-edge rounded-lg border p-4">
            <div class="flex items-start gap-3">
              <TriangleAlertIcon class="text-warn mt-0.5 size-5" />
              <div>
                <p class="text-primary text-label">High memory usage</p>
                <p class="text-secondary text-copy-sm mt-1">
                  Your app is using 89% of allocated memory.
                </p>
              </div>
            </div>
          </div>

          <div class="bg-accent-subtle border-edge rounded-lg border p-4">
            <div class="flex items-start gap-3">
              <InfoIcon class="text-accent mt-0.5 size-5" />
              <div>
                <p class="text-primary text-label">New feature available</p>
                <p class="text-secondary text-copy-sm mt-1">
                  Database backups are now available.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!--
        SECTION 13: TOKEN REFERENCE
      -->
      <section class="pb-16">
        <div class="mb-6">
          <h2 class="text-primary text-title-sm mb-1">Token Reference</h2>
          <p class="text-secondary text-copy">
            Complete list of tokens with derivation formulas.
          </p>
        </div>

        <div class="border-edge overflow-hidden rounded-lg border">
          <table class="w-full">
            <thead>
              <tr class="bg-surface-1 border-edge border-b">
                <th class="text-secondary text-copy-sm px-4 py-2 text-left font-medium">Token</th>
                <th class="text-secondary text-copy-sm px-4 py-2 text-left font-medium">Class</th>
                <th class="text-secondary text-copy-sm px-4 py-2 text-left font-medium">Derives from</th>
                <th class="text-secondary text-copy-sm px-4 py-2 text-left font-medium">Swatch</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-edge">
              <tr v-for="b in baseLevels" :key="b.token" class="hover:bg-surface-1">
                <td class="text-primary text-copy-sm px-4 py-2 font-mono">{{ b.token }}</td>
                <td class="text-secondary text-copy-sm px-4 py-2">.{{ b.class }}</td>
                <td class="text-tertiary text-copy-xs px-4 py-2 font-mono">--seed (fixed)</td>
                <td class="px-4 py-2">
                  <div :class="b.class" class="border-edge size-6 rounded border"></div>
                </td>
              </tr>
              <tr v-for="s in surfaceLevels" :key="s.token" class="hover:bg-surface-1">
                <td class="text-primary text-copy-sm px-4 py-2 font-mono">{{ s.token }}</td>
                <td class="text-secondary text-copy-sm px-4 py-2">{{ s.class }}</td>
                <td class="text-tertiary text-copy-xs px-4 py-2 font-mono">--base (context)</td>
                <td class="px-4 py-2">
                  <div :class="s.class" class="border-edge size-6 rounded border"></div>
                </td>
              </tr>
              <tr v-for="t in textLevels" :key="t.token" class="hover:bg-surface-1">
                <td class="text-primary text-copy-sm px-4 py-2 font-mono">{{ t.token }}</td>
                <td class="text-secondary text-copy-sm px-4 py-2">{{ t.class }}</td>
                <td class="text-tertiary text-copy-xs px-4 py-2 font-mono">--base (context)</td>
                <td class="px-4 py-2">
                  <div class="border-edge flex size-6 items-center justify-center rounded border">
                    <span :class="t.class" class="text-xs font-bold">A</span>
                  </div>
                </td>
              </tr>
              <tr v-for="e in edgeLevels" :key="e.token" class="hover:bg-surface-1">
                <td class="text-primary text-copy-sm px-4 py-2 font-mono">{{ e.token }}</td>
                <td class="text-secondary text-copy-sm px-4 py-2">{{ e.class }}</td>
                <td class="text-tertiary text-copy-xs px-4 py-2 font-mono">--base (context)</td>
                <td class="px-4 py-2">
                  <div :class="e.class" class="size-6 rounded border-2"></div>
                </td>
              </tr>
              <tr v-for="i in intents" :key="i.name" class="hover:bg-surface-1">
                <td class="text-primary text-copy-sm px-4 py-2 font-mono">--color-{{ i.name }}</td>
                <td class="text-secondary text-copy-sm px-4 py-2">{{ i.base }}</td>
                <td class="text-tertiary text-copy-xs px-4 py-2 font-mono">own seed (global)</td>
                <td class="px-4 py-2">
                  <div class="flex gap-1">
                    <div :class="i.subtle" class="size-6 rounded"></div>
                    <div :class="i.base" class="size-6 rounded"></div>
                    <div :class="i.strong" class="size-6 rounded"></div>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>
