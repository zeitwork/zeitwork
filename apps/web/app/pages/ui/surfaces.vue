<script setup lang="ts">
import { usePreferredDark } from "@vueuse/core";
import {
  GitBranchIcon,
  GitCommitHorizontalIcon,
  EllipsisIcon,
} from "lucide-vue-next";

definePageMeta({ layout: "auth" });

const prefersDark = usePreferredDark();

// One step per base level
const base0Step = ref(0.035);
const base1Step = ref(0.035);
const base2Step = ref(0.035);
const base3Step = ref(0.035);

// Sync defaults based on system preference
watch(prefersDark, (dark) => {
  if (dark) {
    base0Step.value = 0.14;
    base1Step.value = 0.065;
    base2Step.value = 0.05;
    base3Step.value = 0.04;
  } else {
    base0Step.value = 0.035;
    base1Step.value = 0.035;
    base2Step.value = 0.035;
    base3Step.value = 0.035;
  }
}, { immediate: true });

// Apply step overrides to documentElement
const overrideProps = ["--surface-step-on-0", "--surface-step-on-1", "--surface-step-on-2", "--surface-step-on-3"] as const;

function applySteps() {
  if (!import.meta.client) return;
  const root = document.documentElement;
  root.style.setProperty("--surface-step-on-0", String(base0Step.value));
  root.style.setProperty("--surface-step-on-1", String(base1Step.value));
  root.style.setProperty("--surface-step-on-2", String(base2Step.value));
  root.style.setProperty("--surface-step-on-3", String(base3Step.value));
}

watch([base0Step, base1Step, base2Step, base3Step], applySteps, { immediate: true });
onMounted(applySteps);

onUnmounted(() => {
  if (!import.meta.client) return;
  const root = document.documentElement;
  for (const prop of overrideProps) {
    root.style.removeProperty(prop);
  }
});

type BaseConfig = {
  label: string;
  class: string;
  stepRef: Ref<number>;
};

const bases: BaseConfig[] = [
  { label: "base-0", class: "base-0", stepRef: base0Step },
  { label: "base-1", class: "base-1", stepRef: base1Step },
  { label: "base-2", class: "base-2", stepRef: base2Step },
  { label: "base-3", class: "base-3", stepRef: base3Step },
];
</script>

<template>
  <div class="base-2 min-h-screen">
    <div class="border-edge border-b">
      <div class="mx-auto max-w-7xl px-6 py-6">
        <h1 class="text-primary text-title-sm mb-1">Surface Tuner</h1>
        <p class="text-secondary text-copy">
          One step per base. Surfaces = step &times; 1, step &times; 2, step &times; 3.
        </p>
      </div>
    </div>

    <div class="mx-auto max-w-7xl space-y-12 px-6 py-8">

      <section v-for="b in bases" :key="b.label">
        <div class="mb-4 flex items-center gap-6">
          <h2 class="text-primary text-title-sm">{{ b.label }}</h2>
          <div class="flex flex-1 items-center gap-3">
            <label class="text-tertiary text-copy-xs">Step</label>
            <input
              type="range" min="0" max="0.2" step="0.005"
              v-model.number="b.stepRef.value"
              class="h-1.5 w-60 cursor-pointer appearance-none rounded-full bg-surface-2"
            />
            <span class="text-primary text-copy-xs font-mono w-12">{{ b.stepRef.value }}</span>
            <span class="text-tertiary text-copy-xs font-mono">
              &times;1={{ b.stepRef.value }}
              &times;2={{ (b.stepRef.value * 2).toFixed(3) }}
              &times;3={{ (b.stepRef.value * 3).toFixed(3) }}
            </span>
          </div>
        </div>

        <div :class="b.class" class="overflow-hidden rounded-xl ring-1 ring-edge">
          <div class="flex items-center gap-3 p-5">
            <div class="rounded-lg px-4 py-3">
              <p class="text-primary text-copy-xs font-mono">base</p>
            </div>
            <div class="bg-surface-1 rounded-lg px-4 py-3">
              <p class="text-primary text-copy-xs font-mono">s-1</p>
            </div>
            <div class="bg-surface-2 rounded-lg px-4 py-3">
              <p class="text-primary text-copy-xs font-mono">s-2</p>
            </div>
            <div class="bg-surface-3 rounded-lg px-4 py-3">
              <p class="text-primary text-copy-xs font-mono">s-3</p>
            </div>
          </div>

          <div class="border-edge border-t">
            <div class="border-edge flex items-center gap-3 border-b px-5 py-3 transition-colors hover:bg-surface-1">
              <div class="bg-accent size-8 rounded-md"></div>
              <div>
                <p class="text-primary text-copy-sm">my-app</p>
                <p class="text-tertiary text-copy-xs">hover:bg-surface-1</p>
              </div>
            </div>
            <div class="border-edge flex items-center gap-3 border-b px-5 py-3 transition-colors hover:bg-surface-1">
              <div class="bg-accent size-8 rounded-md"></div>
              <div>
                <p class="text-primary text-copy-sm">api-service</p>
                <p class="text-tertiary text-copy-xs">hover:bg-surface-1</p>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-1 p-3">
            <DHover background="bg-inverse/5">
              <div class="text-secondary text-copy flex h-8 items-center gap-0.5 px-2">Overview</div>
            </DHover>
            <DHover :active="true" background="bg-inverse/5">
              <div class="text-primary text-copy flex h-8 items-center gap-0.5 px-2">Deployments</div>
            </DHover>
            <DHover background="bg-inverse/5">
              <div class="text-secondary text-copy flex h-8 items-center gap-0.5 px-2">Logs</div>
            </DHover>
          </div>

          <div class="border-edge flex items-center gap-3 border-t p-5">
            <DInput type="email" placeholder="you@example.com" />
            <DButton variant="primary" size="sm">Save</DButton>
            <DButton variant="secondary" size="sm">Cancel</DButton>
          </div>
        </div>
      </section>

      <!-- Full layout replica -->
      <section class="pb-12">
        <h2 class="text-primary text-title-sm mb-2">Layout replica</h2>

        <div class="base-0 flex h-[600px] flex-col overflow-hidden rounded-xl">
          <DNavbarHeader class="shrink-0" />
          <div class="flex flex-1 flex-col px-1 pb-1">
            <div class="base-1 flex min-h-0 flex-1 flex-col rounded-lg ring-1 ring-edge">
              <DNavbar :links="[
                { name: 'Deployments', to: '#', active: true },
                { name: 'Settings', to: '#' },
              ]" />
              <div class="base-2 flex-1 overflow-auto rounded-lg ring-1 ring-edge">
                <div class="flex items-center justify-between p-2">
                  <div class="flex items-center gap-2">
                    <span class="text-primary text-copy-sm px-2 py-1.5 font-medium">Deployments</span>
                    <span class="text-tertiary text-copy-sm">Automatically created for pushes to</span>
                    <span class="text-primary text-copy-xs font-mono">dokedu/dokedu</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <DInput placeholder="All Branches..." />
                    <DInput placeholder="All Environments" />
                  </div>
                </div>

                <DList>
                  <DListItem>
                    <div class="flex w-full items-center">
                      <div class="flex w-[160px] shrink-0 flex-col gap-1">
                        <span class="text-primary text-copy-sm font-mono">EjnklDBk2</span>
                        <span class="text-tertiary text-copy-xs">Preview</span>
                      </div>
                      <div class="flex w-[100px] shrink-0 items-center gap-1.5">
                        <div class="size-2 rounded-full bg-orange-400"></div>
                        <span class="text-tertiary text-copy-sm">Building</span>
                      </div>
                      <div class="flex min-w-0 flex-1 items-center gap-2">
                        <GitBranchIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-sm">dev</span>
                        <GitCommitHorizontalIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-xs font-mono">5315ffa</span>
                        <span class="text-primary text-copy-sm truncate">Add validation for document fields</span>
                      </div>
                      <div class="flex w-[200px] shrink-0 items-center justify-end gap-1.5">
                        <span class="text-tertiary text-copy-xs">25s ago</span>
                        <span class="text-tertiary text-copy-xs">aaronmahlke</span>
                        <DButton variant="transparent" :icon-left="EllipsisIcon" size="sm" />
                      </div>
                    </div>
                  </DListItem>
                  <DListItem>
                    <div class="flex w-full items-center">
                      <div class="flex w-[160px] shrink-0 flex-col gap-1">
                        <span class="text-primary text-copy-sm font-mono">EjnklDBk2</span>
                        <span class="text-tertiary text-copy-xs">Production</span>
                      </div>
                      <div class="flex w-[100px] shrink-0 items-center gap-1.5">
                        <div class="size-2 rounded-full bg-green-500"></div>
                        <span class="text-tertiary text-copy-sm">Ready</span>
                      </div>
                      <div class="flex min-w-0 flex-1 items-center gap-2">
                        <GitBranchIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-sm font-mono">main</span>
                        <GitCommitHorizontalIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-xs font-mono">5315ffa</span>
                        <span class="text-primary text-copy-sm truncate">Add export functionality for entries</span>
                      </div>
                      <div class="flex w-[200px] shrink-0 items-center justify-end gap-1.5">
                        <span class="text-tertiary text-copy-xs">5d ago</span>
                        <span class="text-tertiary text-copy-xs">aaronmahlke</span>
                        <DButton variant="transparent" :icon-left="EllipsisIcon" size="sm" />
                      </div>
                    </div>
                  </DListItem>
                  <DListItem>
                    <div class="flex w-full items-center">
                      <div class="flex w-[160px] shrink-0 flex-col gap-1">
                        <span class="text-primary text-copy-sm font-mono">EjnklDBk2</span>
                        <span class="text-tertiary text-copy-xs">Preview</span>
                      </div>
                      <div class="flex w-[100px] shrink-0 items-center gap-1.5">
                        <div class="size-2 rounded-full bg-green-500"></div>
                        <span class="text-tertiary text-copy-sm">Ready</span>
                      </div>
                      <div class="flex min-w-0 flex-1 items-center gap-2">
                        <GitBranchIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-sm">dev</span>
                        <GitCommitHorizontalIcon class="text-tertiary size-3.5" />
                        <span class="text-tertiary text-copy-xs font-mono">5315ffa</span>
                        <span class="text-primary text-copy-sm truncate">Refactor reports page</span>
                      </div>
                      <div class="flex w-[200px] shrink-0 items-center justify-end gap-1.5">
                        <span class="text-tertiary text-copy-xs">25 ago</span>
                        <span class="text-tertiary text-copy-xs">aaronmahlke</span>
                        <DButton variant="transparent" :icon-left="EllipsisIcon" size="sm" />
                      </div>
                    </div>
                  </DListItem>
                </DList>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
