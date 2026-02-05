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
  <div class="p-4">
    <div class="space-y-6">
      <div>
        <h3 class="text-neutral-strong mb-2 text-sm font-medium">Project Name</h3>
        <input
          type="text"
          class="bg-neutral-weak border-neutral-subtle text-neutral-strong placeholder-neutral-moderate focus:border-neutral-moderate focus:ring-neutral-moderate w-full rounded-md border px-3 py-2 text-sm focus:ring-1 focus:outline-none"
          :value="projectSlug"
          disabled
        />
      </div>

      <div>
        <h3 class="text-neutral-strong mb-2 text-sm font-medium">Organization</h3>
        <input
          type="text"
          class="bg-neutral-weak border-neutral-subtle text-neutral-strong placeholder-neutral-moderate focus:border-neutral-moderate focus:ring-neutral-moderate w-full rounded-md border px-3 py-2 text-sm focus:ring-1 focus:outline-none"
          :value="orgId"
          disabled
        />
      </div>

      <div>
        <h3 class="text-neutral-strong mb-2 text-sm font-medium">Root Directory</h3>
        <p class="text-neutral-moderate mb-2 text-xs">
          The directory where your Dockerfile is located. Use "/" for repository root, or specify a
          subdirectory like "/apps/web" for monorepos.
        </p>
        <input
          type="text"
          v-model="rootDirectory"
          placeholder="/"
          class="bg-neutral-weak border-neutral-subtle text-neutral-strong placeholder-neutral-moderate focus:border-neutral-moderate focus:ring-neutral-moderate w-full rounded-md border px-3 py-2 text-sm focus:ring-1 focus:outline-none"
        />
      </div>

      <div class="flex items-center gap-3">
        <DButton variant="primary" :loading="isSaving" @click="saveSettings">
          Save Changes
        </DButton>
        <p v-if="saveMessage" :class="saveMessage.type === 'error' ? 'text-red-500' : 'text-green-500'" class="text-sm">
          {{ saveMessage.text }}
        </p>
      </div>
    </div>
  </div>
</template>
