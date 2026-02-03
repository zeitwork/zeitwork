<script setup lang="ts">
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

// const { isPending, start, stop } = useTimeoutFn(() => {
//   refreshDeployments();
// }, 1000);

// start();

// watch(isPending, (newVal) => {
//   if (newVal) {
//     start();
//   } else {
//     start();
//   }
// });

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
    case "ready":
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
    case "ready":
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
        <p class="text-copy text-neutral-subtle">Automatically created for pushes to</p>
        <a
          :href="`https://github.com/${project?.githubRepository}`"
          target="_blank"
          rel="noopener noreferrer"
          class="text-copy text-neutral flex items-center gap-1 hover:underline"
        >
          <img src="/icons/github-mark.svg" class="size-4" />
          <span>{{ project?.githubRepository }}</span>
        </a>
      </template>
      <template #trailing>
        <DButton @click="createDeployment" :loading="isCreatingDeployment">
          Create Deployment
        </DButton>
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
          <span>domains</span>
        </div>
        <div class="text-right">{{ renderDate(deployment.createdAt) }}</div>
      </nuxt-link>
    </div>
  </div>
</template>
