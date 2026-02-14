<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";
import {
  CheckCircle2Icon,
  ChevronDownIcon,
  ChevronRightIcon,
  CircleDotIcon,
  HammerIcon,
  LoaderIcon,
  TerminalIcon,
} from "lucide-vue-next";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const projectSlug = route.params.project as string;
const deploymentId = route.params.id as string;

const formattedId = computed(() => uuidToB58(deploymentId));

const { data: deployment, refresh: refreshDeployment } = await useFetch(
  `/api/deployments/${deploymentId}`,
);

const { data: buildLogs, refresh: refreshBuildLogs } = await useFetch(
  `/api/logs`,
  {
    query: {
      projectSlug,
      deploymentId,
    },
  },
);

const isBuilding = computed(
  () => !deployment.value || deployment.value.status === "building" || deployment.value.status === "pending",
);

const isSettled = computed(
  () => deployment.value && ["running", "stopped", "failed"].includes(deployment.value.status),
);

// --- VM Logs (cursor-based accumulation) ---
const vmLogEntries = ref<Array<{ id: string; message: string; level: string | null }>>([]);
const vmLogCursor = ref<string | null>(null);
const isLoadingVmLogs = ref(false);

const hasVmLogs = computed(() => {
  if (!deployment.value) return false;
  const status = deployment.value.status;
  return ["starting", "running", "stopped", "failed"].includes(status);
});

const isVmLogStreaming = computed(() => {
  if (!deployment.value) return false;
  return ["starting", "running"].includes(deployment.value.status);
});

async function fetchVmLogs() {
  if (isLoadingVmLogs.value) return;
  isLoadingVmLogs.value = true;
  try {
    const params: Record<string, string> = {};
    if (vmLogCursor.value) {
      params.cursor = vmLogCursor.value;
    }
    const result = await $fetch(`/api/deployments/${deploymentId}/logs`, {
      query: params,
    });
    if (result.logs && result.logs.length > 0) {
      vmLogEntries.value.push(...result.logs);
      const lastLog = result.logs.at(-1);
      if (lastLog) {
        vmLogCursor.value = lastLog.id;
      }
    }
  } catch (error) {
    console.error("Failed to fetch VM logs:", error);
  } finally {
    isLoadingVmLogs.value = false;
  }
}

// Initial fetch
if (hasVmLogs.value) {
  await fetchVmLogs();
}

const parsedVmLogs = computed(() =>
  vmLogEntries.value.map((log) => parseAnsi(log.message)),
);

useIntervalFn(() => {
  if (!isSettled.value) {
    refreshDeployment();
  }
  if (isBuilding.value) {
    refreshBuildLogs();
  }
  if (hasVmLogs.value) {
    fetchVmLogs();
  }
}, 1000);

function deploymentStatusColor(status: string) {
  switch (status) {
    case "pending":
      return "text-warn";
    case "building":
      return "text-accent";
    case "starting":
      return "text-accent";
    case "running":
      return "text-success";
    case "failed":
      return "text-danger";
    case "stopped":
      return "text-tertiary";
    default:
      return "text-tertiary";
  }
}

function deploymentStatusBgColor(status: string) {
  switch (status) {
    case "pending":
      return "bg-warn-subtle";
    case "building":
      return "bg-accent-subtle";
    case "starting":
      return "bg-accent-subtle";
    case "running":
      return "bg-success-subtle";
    case "failed":
      return "bg-danger-subtle";
    case "stopped":
      return "bg-inverse/5";
    default:
      return "bg-inverse/5";
  }
}

// --- View mode toggle ---
const viewMode = ref<"refined" | "raw">("refined");

// --- Parsed ANSI for raw view ---
const parsedBuildLogs = computed(
  () => buildLogs.value?.map((log) => parseAnsi(log.message)) ?? [],
);

// --- Structured sections for refined view ---
const sections = computed(() =>
  parseBuildLogSections(
    buildLogs.value?.map((l) => ({ message: l.message, level: l.level })) ?? [],
    { isBuilding: isBuilding.value },
  ),
);

// --- Section collapse state ---
// Key: section index, Value: true = expanded, false = collapsed
const sectionToggle = ref<Record<number, boolean>>({});
const previousRunningSection = ref<number>(-1);

watch(
  sections,
  (newSections) => {
    const runningIdx = newSections.findIndex((s) => s.status === "running");
    if (runningIdx !== -1 && runningIdx !== previousRunningSection.value) {
      sectionToggle.value[runningIdx] = true;
      previousRunningSection.value = runningIdx;
    }
  },
  { immediate: true },
);

function isSectionExpanded(index: number): boolean {
  if (sectionToggle.value[index] !== undefined) {
    return sectionToggle.value[index];
  }
  const section = sections.value[index];
  if (!section) return false;
  if (section.status === "running") return true;
  if (index === sections.value.length - 1) return true;
  return false;
}

function toggleSection(index: number) {
  sectionToggle.value[index] = !isSectionExpanded(index);
}

// --- Step collapse state ---
// Key: "sectionIndex-stepIndex", Value: true = expanded, false = collapsed
const stepToggle = ref<Record<string, boolean>>({});

function isStepExpanded(sIndex: number, stepIndex: number): boolean {
  const key = `${sIndex}-${stepIndex}`;
  if (stepToggle.value[key] !== undefined) {
    return stepToggle.value[key];
  }
  const section = sections.value[sIndex];
  if (!section) return false;
  const step = section.steps[stepIndex];
  if (!step) return false;
  // Auto-expand running steps, collapse completed ones
  if (step.status === "running") return true;
  return false;
}

function toggleStep(sIndex: number, stepIndex: number) {
  const key = `${sIndex}-${stepIndex}`;
  stepToggle.value[key] = !isStepExpanded(sIndex, stepIndex);
}
</script>

<template>
  <div class="h-full flex flex-col flex-1">
    <DHeader title="Deployment details">
      <template #leading>
        <span class="text-copy text-secondary font-mono">{{
          formattedId
        }}</span>
      </template>
      <template #trailing>
        <div
          v-if="deployment"
          :class="[
            deploymentStatusColor(deployment.status),
            deploymentStatusBgColor(deployment.status),
            'rounded px-1.5 py-0.5 text-xs',
          ]"
        >
          {{ deployment.status }}
        </div>
      </template>
    </DHeader>

    <div class="flex-1 overflow-auto">
      <!-- Build Logs -->
      <div class="border-edge-subtle border-b">
        <!-- Header with view toggle -->
        <div
          class="border-edge-subtle flex items-center gap-2 border-b bg-surface-1 px-4 py-2"
        >
          <HammerIcon class="size-4 text-secondary" />
          <span class="text-sm font-medium text-secondary">Build Logs</span>

          <div class="ml-auto flex items-center rounded-md border border-edge-subtle overflow-hidden">
            <button
              class="px-2 py-0.5 text-xs transition-colors"
              :class="
                viewMode === 'refined'
                  ? 'bg-surface-2 text-secondary font-medium'
                  : 'text-tertiary hover:text-secondary'
              "
              @click="viewMode = 'refined'"
            >
              Refined
            </button>
            <button
              class="px-2 py-0.5 text-xs transition-colors border-l border-edge-subtle"
              :class="
                viewMode === 'raw'
                  ? 'bg-surface-2 text-secondary font-medium'
                  : 'text-tertiary hover:text-secondary'
              "
              @click="viewMode = 'raw'"
            >
              Raw
            </button>
          </div>
        </div>

        <!-- Empty state -->
        <div
          v-if="!buildLogs || buildLogs.length === 0"
          class="bg-surface-0 p-4 font-mono text-sm"
        >
          <pre class="text-xs text-tertiary">No build logs available yet...</pre>
        </div>

        <!-- Raw view -->
        <div
          v-else-if="viewMode === 'raw'"
          class="bg-surface-0 p-4 font-mono"
        >
          <pre
            v-for="(segments, index) in parsedBuildLogs"
            :key="index"
            class="text-xs text-secondary"
          ><span
              v-for="(segment, i) in segments"
              :key="i"
              :style="{
                color: segment.color ?? undefined,
                fontWeight: segment.bold ? 'bold' : undefined,
              }"
            >{{ segment.text }}</span></pre>
        </div>

        <!-- Refined view -->
        <div v-else>
          <div
            v-for="(section, sIndex) in sections"
            :key="sIndex"
            class="border-edge-subtle border-b last:border-b-0"
          >
            <!-- Section header -->
            <button
              class="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-surface-1 transition-colors"
              @click="toggleSection(sIndex)"
            >
              <ChevronDownIcon
                v-if="isSectionExpanded(sIndex)"
                class="size-3.5 text-tertiary shrink-0"
              />
              <ChevronRightIcon
                v-else
                class="size-3.5 text-tertiary shrink-0"
              />

              <CheckCircle2Icon
                v-if="section.status === 'completed'"
                class="size-3.5 text-green-500 shrink-0"
              />
              <LoaderIcon
                v-else-if="section.status === 'running'"
                class="size-3.5 text-blue-400 shrink-0 animate-spin"
              />
              <CircleDotIcon
                v-else
                class="size-3.5 text-tertiary shrink-0"
              />

              <span class="text-xs font-medium text-secondary">{{
                section.name
              }}</span>

              <span
                v-if="section.duration !== null"
                class="ml-auto text-xs font-mono text-tertiary"
              >
                {{ formatDuration(section.duration) }}
              </span>
            </button>

            <!-- Section content: steps -->
            <div
              v-if="isSectionExpanded(sIndex)"
              class="bg-surface-0 font-mono"
            >
              <div
                v-for="(step, stepIndex) in section.steps"
                :key="stepIndex"
                class="border-edge-subtle border-t"
              >
                <!-- Step header row (clickable if has output) -->
                <button
                  v-if="step.outputLines.length > 0"
                  class="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-surface-1/50 transition-colors"
                  @click="toggleStep(sIndex, stepIndex)"
                >
                  <ChevronDownIcon
                    v-if="isStepExpanded(sIndex, stepIndex)"
                    class="size-3.5 text-tertiary/50 shrink-0"
                  />
                  <ChevronRightIcon
                    v-else
                    class="size-3.5 text-tertiary/50 shrink-0"
                  />

                  <CheckCircle2Icon
                    v-if="step.status === 'completed'"
                    class="size-3.5 text-green-500/60 shrink-0"
                  />
                  <LoaderIcon
                    v-else
                    class="size-3.5 text-blue-400/60 shrink-0 animate-spin"
                  />

                  <span class="text-xs text-tertiary truncate">{{
                    step.command
                  }}</span>

                  <span
                    v-if="step.duration !== null"
                    class="ml-auto text-xs font-mono text-tertiary shrink-0"
                  >
                    {{ formatDuration(step.duration) }}
                  </span>
                </button>

                <!-- Step header row (non-clickable, no output) -->
                <div
                  v-else
                  class="flex items-center gap-2 px-4 py-2"
                >
                  <!-- Spacer matching chevron width -->
                  <div class="size-3.5 shrink-0" />

                  <CheckCircle2Icon
                    v-if="step.status === 'completed'"
                    class="size-3.5 text-green-500/60 shrink-0"
                  />
                  <LoaderIcon
                    v-else
                    class="size-3.5 text-blue-400/60 shrink-0 animate-spin"
                  />

                  <span class="text-xs text-tertiary truncate">{{
                    step.command
                  }}</span>

                  <span
                    v-if="step.duration !== null"
                    class="ml-auto text-xs font-mono text-tertiary shrink-0"
                  >
                    {{ formatDuration(step.duration) }}
                  </span>
                </div>

                <!-- Step output lines (collapsible) -->
                <div
                  v-if="
                    step.outputLines.length > 0 &&
                    isStepExpanded(sIndex, stepIndex)
                  "
                  class="py-2"
                >
                  <pre
                    v-for="(line, lIndex) in step.outputLines"
                    :key="lIndex"
                    class="text-xs text-secondary pl-[60px]"
                  ><span
                      v-for="(segment, i) in parseAnsi(line.message)"
                      :key="i"
                      :style="{
                        color: segment.color ?? undefined,
                        fontWeight: segment.bold ? 'bold' : undefined,
                      }"
                    >{{ segment.text }}</span></pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Runtime Logs -->
      <div v-if="hasVmLogs" class="border-edge-subtle border-b">
        <div
          class="border-edge-subtle flex items-center gap-2 border-b bg-surface-1 px-4 py-2"
        >
          <TerminalIcon class="size-4 text-secondary" />
          <span class="text-sm font-medium text-secondary">Runtime Logs</span>
          <LoaderIcon
            v-if="isVmLogStreaming"
            class="size-3.5 text-tertiary animate-spin"
          />
        </div>

        <div
          v-if="vmLogEntries.length === 0"
          class="bg-surface-0 p-4 font-mono text-sm"
        >
          <pre class="text-xs text-tertiary">No runtime logs available yet...</pre>
        </div>

        <div v-else class="bg-surface-0 p-4 font-mono">
          <pre
            v-for="(segments, index) in parsedVmLogs"
            :key="index"
            class="text-xs text-secondary"
          ><span
              v-for="(segment, i) in segments"
              :key="i"
              :style="{
                color: segment.color ?? undefined,
                fontWeight: segment.bold ? 'bold' : undefined,
              }"
            >{{ segment.text }}</span></pre>
        </div>
      </div>
    </div>
  </div>
</template>
