<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";

definePageMeta({
  layout: "project",
});

const route = useRoute();
const orgId = route.params.org as string;
const projectSlug = route.params.project as string;

const { data: project } = await useFetch(`/api/projects/${projectSlug}`);

const { data: deployments, refresh: refreshDeployments } = await useFetch(
  `/api/projects/${projectSlug}/deployments`,
);

useIntervalFn(() => {
  refreshDeployments();
}, 1000);

const isCreatingDeployment = ref(false);

async function createDeployment() {
  isCreatingDeployment.value = true;
  try {
    await $fetch(`/api/projects/${projectSlug}/deployments`, {
      method: "POST",
    });
    await refreshDeployments();
  } catch (error) {
    console.error("Failed to create deployment:", error);
  } finally {
    isCreatingDeployment.value = false;
  }
}

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
      return "text-yellow-500";
    case "building":
      return "text-blue-500";
    case "starting":
      return "text-blue-500";
    case "running":
      return "text-green-500";
    case "failed":
      return "text-red-500";
    case "inactive":
      return "text-neutral-subtle";
    default:
      return "text-neutral";
  }
}

function deploymentStatusBgColor(status: string) {
  switch (status) {
    case "pending":
      return "bg-yellow-100";
    case "building":
      return "bg-blue-100";
    case "starting":
      return "bg-blue-100";
    case "running":
      return "bg-green-100";
    case "failed":
      return "bg-red-100";
    case "inactive":
      return "bg-neutral-subtle";
    default:
      return "bg-neutral/10";
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
        <span class="text-copy text-neutral-subtle">Automatically created for pushes to</span>
        <a
          v-if="project?.githubRepository"
          :href="`https://github.com/${project.githubRepository}`"
          target="_blank"
          class="text-neutral flex items-center gap-1"
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
    <div class="flex-1 overflow-auto">
      <nuxt-link
        v-for="deployment in deployments"
        :key="deployment.id"
        :to="deploymentLink(deployment)"
        class="hover:bg-surface-subtle border-neutral-subtle text-neutral grid cursor-pointer grid-cols-[auto_100px_100px_3fr_1fr] items-center gap-4 border-b p-4 text-sm"
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
        <div>
          <template v-if="deployment.domains?.length">
            <a
              v-for="(domain, idx) in deployment.domains"
              :key="domain"
              :href="`https://${domain}`"
              target="_blank"
              class="text-blue-500 hover:underline"
              @click.stop
            >{{ domain }}<span v-if="idx < deployment.domains.length - 1" class="text-neutral-subtle">, </span></a>
          </template>
          <span v-else class="text-neutral-subtle">—</span>
        </div>
        <div class="text-right">{{ renderDate(deployment.createdAt) }}</div>
      </nuxt-link>
    </div>
  </div>
</template>
