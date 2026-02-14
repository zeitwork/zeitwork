<script setup lang="ts">
import { useInfiniteScroll, useIntervalFn } from "@vueuse/core";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const orgId = route.params.org as string;
const projectSlug = route.params.project as string;

const { data: project } = await useFetch(`/api/projects/${projectSlug}`);

// --- Deployment list with infinite scroll ---
type Deployment = {
  id: string;
  status: string;
  projectId: string;
  githubCommit: string;
  organisationId: string;
  createdAt: string;
  updatedAt: string;
  domains: string[];
};

const deploymentList = ref<Deployment[]>([]);
const nextCursor = ref<string | null>(null);
const isLoadingMore = ref(false);

async function fetchDeployments(cursor?: string) {
  const params: Record<string, string> = {};
  if (cursor) {
    params.cursor = cursor;
  }
  return await $fetch(`/api/projects/${projectSlug}/deployments`, {
    query: params,
  });
}

// Initial load
const initialData = await fetchDeployments();
deploymentList.value = initialData.deployments;
nextCursor.value = initialData.nextCursor;

// Infinite scroll
const scrollContainer = useTemplateRef<HTMLElement>("scrollContainer");

useInfiniteScroll(
  scrollContainer,
  async () => {
    if (!nextCursor.value || isLoadingMore.value) return;
    isLoadingMore.value = true;
    try {
      const result = await fetchDeployments(nextCursor.value);
      deploymentList.value.push(...result.deployments);
      nextCursor.value = result.nextCursor;
    } catch (error) {
      console.error("Failed to load more deployments:", error);
    } finally {
      isLoadingMore.value = false;
    }
  },
  {
    distance: 100,
    canLoadMore: () => !!nextCursor.value,
  },
);

// Background polling: fetch first page and merge updates
useIntervalFn(async () => {
  try {
    const result = await fetchDeployments();
    const existingIds = new Set(deploymentList.value.map((d) => d.id));

    // Prepend new deployments
    const newDeployments = result.deployments.filter((d) => !existingIds.has(d.id));
    if (newDeployments.length > 0) {
      deploymentList.value.unshift(...newDeployments);
    }

    // Update existing deployments in-place
    for (const fresh of result.deployments) {
      const existing = deploymentList.value.find((d) => d.id === fresh.id);
      if (existing) {
        existing.status = fresh.status;
        existing.updatedAt = fresh.updatedAt;
        existing.domains = fresh.domains;
      }
    }
  } catch {
    // Silently ignore polling errors
  }
}, 1000);

const isCreatingDeployment = ref(false);
const deploymentError = ref<string | null>(null);

async function createDeployment() {
  isCreatingDeployment.value = true;
  deploymentError.value = null;
  try {
    await $fetch(`/api/projects/${projectSlug}/deployments`, {
      method: "POST",
    });
    // Poll will pick up the new deployment
  } catch (error: any) {
    console.error("Failed to create deployment:", error);
    deploymentError.value =
      error?.data?.message || "Failed to create deployment";
  } finally {
    isCreatingDeployment.value = false;
  }
}

const isGitHubError = computed(() => {
  if (!deploymentError.value) return false;
  const msg = deploymentError.value.toLowerCase();
  return (
    msg.includes("github") ||
    msg.includes("installation") ||
    msg.includes("commit")
  );
});

function renderDate(date: string) {
  return Intl.DateTimeFormat("en-DE", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(date));
}

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

function deploymentLink(deployment: any) {
  return `/${orgId}/${projectSlug}/deployments/${deployment.id}`;
}
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <DHeader title="Deployments">
      <template #leading>
        <span class="text-copy text-secondary">Automatically created for pushes to</span>
        <a
          v-if="project?.githubRepository"
          :href="`https://github.com/${project.githubRepository}`"
          target="_blank"
          class="text-primary flex items-center gap-1"
        >
          <Icon name="mdi:github" class="size-3.5" />
          <span class="text-copy hover:underline">{{ project.githubRepository }}</span>
        </a>
      </template>
      <template #trailing>
        <d-button @click="createDeployment" :loading="isCreatingDeployment">
          Create Deployment
        </d-button>
      </template>
    </DHeader>
    <div
      v-if="deploymentError"
      class="bg-danger-subtle text-danger border-edge-subtle flex items-center gap-2 border-b px-4 py-3 text-sm"
    >
      <span>{{ deploymentError }}</span>
      <nuxt-link
        v-if="isGitHubError"
        :to="`/${orgId}/${projectSlug}/settings`"
        class="underline"
      >
        Go to Settings
      </nuxt-link>
      <button
        class="text-danger/60 hover:text-danger ml-auto"
        @click="deploymentError = null"
      >
        Dismiss
      </button>
    </div>
    <div ref="scrollContainer" class="flex-1 overflow-auto">
      <nuxt-link
        v-for="deployment in deploymentList"
        :key="deployment.id"
        :to="deploymentLink(deployment)"
        class="hover:bg-surface-1 border-edge-subtle text-primary grid cursor-pointer grid-cols-[auto_100px_100px_3fr_1fr] items-center gap-4 border-b p-4 text-sm"
      >
        <div class="font-mono">{{ uuidToBase58(deployment.id) }}</div>
        <div>
          <div
            :class="[
              deploymentStatusColor(deployment.status),
              deploymentStatusBgColor(deployment.status),
              'w-fit rounded px-1.5 py-0.5',
            ]"
          >
            {{ deployment.status }}
          </div>
        </div>
        <div class="font-mono">{{ deployment.githubCommit.slice(0, 7) }}</div>
        <div class="line-clamp-1">
          <template v-if="deployment.domains?.length">
            <a
              v-for="(domain, idx) in deployment.domains"
              :key="domain"
              :href="`https://${domain}`"
              target="_blank"
              class="text-secondary hover:text-primary hover:underline"
              @click.stop
              >{{ domain
              }}<span v-if="idx < deployment.domains.length - 1" class="text-secondary"
                >,
              </span></a
            >
          </template>
          <span v-else class="text-secondary">—</span>
        </div>
        <div class="text-right line-clamp-1">{{ renderDate(deployment.createdAt) }}</div>
      </nuxt-link>
    </div>
  </div>
</template>
