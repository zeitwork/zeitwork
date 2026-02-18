<script setup lang="ts">
const route = useRoute();

const orgId = route.params.org as string;
const projectSlug = route.params.project as string;

// Fetch project data
const { data: projectData, refresh } = await useFetch(`/api/projects/${projectSlug}`);
const project = computed(() => projectData.value?.[0]);

const rootDirectory = ref(project.value?.rootDirectory || "/");
const dockerfilePath = ref(project.value?.dockerfilePath || "Dockerfile");
const isSaving = ref(false);
const saveMessage = ref<{ type: "success" | "error"; text: string } | null>(null);

// GitHub connection status (client-side only, non-blocking)
const { data: githubStatus, refresh: refreshGithubStatus, status: githubStatusLoading } = useFetch(
  `/api/projects/${projectSlug}/github-status`,
  { server: false },
);
const isReconnecting = ref(false);
const reconnectMessage = ref<{ type: "success" | "error"; text: string } | null>(null);

const config = useRuntimeConfig();
const githubAppInstallUrl = computed(() => {
  const appName = config.public.appName;
  const redirectUri = `${config.public.appUrl}/auth/github`;
  return `https://github.com/apps/${appName}/installations/new?redirect_uri=${redirectUri}`;
});

async function reconnectGithub() {
  isReconnecting.value = true;
  reconnectMessage.value = null;
  try {
    await $fetch(`/api/projects/${projectSlug}/reconnect-github`, {
      method: "POST",
    });
    reconnectMessage.value = { type: "success", text: "GitHub connection restored" };
    await refreshGithubStatus();
  } catch (error: any) {
    console.error("Failed to reconnect GitHub:", error);
    reconnectMessage.value = {
      type: "error",
      text: error?.data?.message || "Failed to reconnect GitHub",
    };
  } finally {
    isReconnecting.value = false;
  }
}

// Update fields when project data loads
watch(
  () => project.value?.rootDirectory,
  (newVal) => {
    if (newVal) rootDirectory.value = newVal;
  },
);
watch(
  () => project.value?.dockerfilePath,
  (newVal) => {
    if (newVal) dockerfilePath.value = newVal;
  },
);

async function saveSettings() {
  isSaving.value = true;
  saveMessage.value = null;

  try {
    // Normalize rootDirectory
    let normalizedRootDir = rootDirectory.value?.trim() || "/";
    if (normalizedRootDir !== "/" && !normalizedRootDir.startsWith("/")) {
      normalizedRootDir = "/" + normalizedRootDir;
    }

    const normalizedDockerfilePath = dockerfilePath.value?.trim() || "Dockerfile";

    await $fetch(`/api/projects/${projectSlug}`, {
      method: "PATCH",
      body: {
        rootDirectory: normalizedRootDir,
        dockerfilePath: normalizedDockerfilePath,
      },
    });

    saveMessage.value = { type: "success", text: "Settings saved successfully" };
    await refresh();
  } catch (error: any) {
    console.error("Failed to save settings:", error);
    saveMessage.value = {
      type: "error",
      text: error?.data?.message || "Failed to save settings",
    };
  } finally {
    isSaving.value = false;
  }
}
</script>

<template>
  <div class="flex flex-col">
    <DHeader title="General" description="Manage your project settings." />
    <div class="space-y-6 p-4">
      <div>
        <h3 class="text-primary mb-2 text-sm font-medium">Project Name</h3>
        <DInput :model-value="projectSlug" disabled />
      </div>

      <div>
        <h3 class="text-primary mb-2 text-sm font-medium">Organization</h3>
        <DInput :model-value="orgId" disabled />
      </div>

      <div>
        <h3 class="text-primary mb-2 text-sm font-medium">Git Repository</h3>
        <div class="border-edge rounded-lg border p-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <Icon name="mdi:github" class="size-4" />
              <a
                v-if="githubStatus?.repository"
                :href="`https://github.com/${githubStatus.repository}`"
                target="_blank"
                class="text-primary text-sm hover:underline"
              >
                {{ githubStatus.repository }}
              </a>
            </div>
            <div
              v-if="githubStatusLoading === 'pending'"
              class="text-tertiary bg-inverse/5 rounded px-1.5 py-0.5 text-xs"
            >
              Checking...
            </div>
            <div
              v-else-if="githubStatus"
              :class="[
                githubStatus.connected ? 'text-success bg-success-subtle' : 'text-danger bg-danger-subtle',
                'rounded px-1.5 py-0.5 text-xs',
              ]"
            >
              {{ githubStatus.connected ? "Connected" : "Disconnected" }}
            </div>
          </div>
          <div v-if="githubStatus && !githubStatus.connected" class="mt-3">
            <p class="text-danger mb-3 text-xs">
              {{ githubStatus.error || "The GitHub connection for this project is broken. Deployments will fail until reconnected." }}
            </p>
            <div class="flex items-center gap-2">
              <DButton
                variant="primary"
                :loading="isReconnecting"
                @click="reconnectGithub"
              >
                Reconnect
              </DButton>
              <a
                :href="githubAppInstallUrl"
                class="text-secondary hover:text-primary text-xs underline"
              >
                Install GitHub App
              </a>
            </div>
            <p
              v-if="reconnectMessage"
              :class="reconnectMessage.type === 'error' ? 'text-danger' : 'text-success'"
              class="mt-2 text-xs"
            >
              {{ reconnectMessage.text }}
            </p>
          </div>
          <p
            v-if="reconnectMessage && githubStatus?.connected"
            :class="reconnectMessage.type === 'error' ? 'text-danger' : 'text-success'"
            class="mt-2 text-xs"
          >
            {{ reconnectMessage.text }}
          </p>
        </div>
      </div>

      <div>
        <h3 class="text-primary mb-2 text-sm font-medium">Root Directory</h3>
        <p class="text-secondary mb-2 text-xs">
          The directory used as the build context. Use "/" for repository root, or specify a
          subdirectory like "/apps/web" for monorepos.
        </p>
        <DInput v-model="rootDirectory" placeholder="/" />
      </div>

      <div>
        <h3 class="text-primary mb-2 text-sm font-medium">Dockerfile Path</h3>
        <p class="text-secondary mb-2 text-xs">
          Path to the Dockerfile relative to the root directory.
        </p>
        <DInput v-model="dockerfilePath" placeholder="Dockerfile" />
      </div>

      <div class="flex items-center gap-3">
        <DButton variant="primary" :loading="isSaving" @click="saveSettings">
          Save Changes
        </DButton>
        <p v-if="saveMessage" :class="saveMessage.type === 'error' ? 'text-danger' : 'text-success'" class="text-sm">
          {{ saveMessage.text }}
        </p>
      </div>
    </div>
  </div>
</template>
