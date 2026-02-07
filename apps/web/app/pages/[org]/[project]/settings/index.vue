<script setup lang="ts">
const route = useRoute();

const orgId = route.params.org as string;
const projectSlug = route.params.project as string;

// Fetch project data
const { data: projectData, refresh } = await useFetch(`/api/projects/${projectSlug}`);
const project = computed(() => projectData.value?.[0]);

const rootDirectory = ref(project.value?.rootDirectory || "/");
const isSaving = ref(false);
const saveMessage = ref<{ type: "success" | "error"; text: string } | null>(null);

// Update rootDirectory when project data loads
watch(
  () => project.value?.rootDirectory,
  (newVal) => {
    if (newVal) rootDirectory.value = newVal;
  }
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

    await $fetch(`/api/projects/${projectSlug}`, {
      method: "PATCH",
      body: {
        rootDirectory: normalizedRootDir,
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
        <h3 class="text-primary mb-2 text-sm font-medium">Root Directory</h3>
        <p class="text-secondary mb-2 text-xs">
          The directory where your Dockerfile is located. Use "/" for repository root, or specify a
          subdirectory like "/apps/web" for monorepos.
        </p>
        <DInput v-model="rootDirectory" placeholder="/" />
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
