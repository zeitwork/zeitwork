<script setup lang="ts">
import { EyeIcon, EyeOffIcon, PencilIcon, TrashIcon } from "lucide-vue-next";
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
  DialogTrigger,
} from "reka-ui";

const route = useRoute();
const projectSlug = route.params.project as string;

type EnvVar = {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

const { data: envVars, refresh: refreshEnvVars } = await useFetch<EnvVar[]>(
  `/api/projects/${projectSlug}/env`,
);

// State for revealed values (id -> value)
const revealedValues = ref<Record<string, string>>({});
const revealingIds = ref<Set<string>>(new Set());

// Add dialog state
const showAddDialog = ref(false);
const newEnvName = ref("");
const newEnvValue = ref("");
const isAdding = ref(false);

// Edit dialog state
const showEditDialog = ref(false);
const editingEnvVar = ref<EnvVar | null>(null);
const editEnvName = ref("");
const editEnvValue = ref("");
const isEditing = ref(false);

// Delete confirmation state
const showDeleteDialog = ref(false);
const deletingEnvVar = ref<EnvVar | null>(null);
const isDeleting = ref(false);

async function revealValue(envVar: EnvVar) {
  if (revealedValues.value[envVar.id]) {
    // Hide the value
    delete revealedValues.value[envVar.id];
    return;
  }

  revealingIds.value.add(envVar.id);
  try {
    const result = await $fetch<{ value: string }>(
      `/api/projects/${projectSlug}/env/${envVar.id}/reveal`,
    );
    revealedValues.value[envVar.id] = result.value;
  } catch (error) {
    console.error("Failed to reveal value:", error);
  } finally {
    revealingIds.value.delete(envVar.id);
  }
}

async function addEnvVar() {
  if (!newEnvName.value.trim()) return;

  isAdding.value = true;
  try {
    await $fetch(`/api/projects/${projectSlug}/env`, {
      method: "POST",
      body: {
        name: newEnvName.value.trim(),
        value: newEnvValue.value,
      },
    });
    newEnvName.value = "";
    newEnvValue.value = "";
    showAddDialog.value = false;
    await refreshEnvVars();
  } catch (error: any) {
    console.error("Failed to add environment variable:", error);
    alert(error.data?.message || "Failed to add environment variable");
  } finally {
    isAdding.value = false;
  }
}

function openEditDialog(envVar: EnvVar) {
  editingEnvVar.value = envVar;
  editEnvName.value = envVar.name;
  editEnvValue.value = "";
  showEditDialog.value = true;
}

async function updateEnvVar() {
  if (!editingEnvVar.value || !editEnvName.value.trim()) return;

  isEditing.value = true;
  try {
    const body: { name?: string; value?: string } = {};

    if (editEnvName.value.trim() !== editingEnvVar.value.name) {
      body.name = editEnvName.value.trim();
    }

    if (editEnvValue.value) {
      body.value = editEnvValue.value;
    }

    if (Object.keys(body).length === 0) {
      showEditDialog.value = false;
      return;
    }

    await $fetch(`/api/projects/${projectSlug}/env/${editingEnvVar.value.id}`, {
      method: "PUT",
      body,
    });

    // Clear revealed value if it was updated
    if (body.value) {
      delete revealedValues.value[editingEnvVar.value.id];
    }

    showEditDialog.value = false;
    editingEnvVar.value = null;
    await refreshEnvVars();
  } catch (error: any) {
    console.error("Failed to update environment variable:", error);
    alert(error.data?.message || "Failed to update environment variable");
  } finally {
    isEditing.value = false;
  }
}

function openDeleteDialog(envVar: EnvVar) {
  deletingEnvVar.value = envVar;
  showDeleteDialog.value = true;
}

async function deleteEnvVar() {
  if (!deletingEnvVar.value) return;

  isDeleting.value = true;
  try {
    await $fetch(`/api/projects/${projectSlug}/env/${deletingEnvVar.value.id}`, {
      method: "DELETE",
    });

    // Clear revealed value
    delete revealedValues.value[deletingEnvVar.value.id];

    showDeleteDialog.value = false;
    deletingEnvVar.value = null;
    await refreshEnvVars();
  } catch (error: any) {
    console.error("Failed to delete environment variable:", error);
    alert(error.data?.message || "Failed to delete environment variable");
  } finally {
    isDeleting.value = false;
  }
}
</script>

<template>
  <div class="flex flex-col">
    <!-- Header -->
    <div class="border-neutral-subtle flex items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Environment Variables</div>
      <DDialog>
        <template #trigger>
          <d-button>Add Variable</d-button>
        </template>
        <template #title>Add Environment Variable</template>
        <template #description>
          Add a new environment variable to your project. Values are encrypted at rest.
        </template>
        <template #content>
          <div class="flex flex-col gap-4">
            <div class="flex flex-col gap-1">
              <label class="text-neutral-strong text-sm font-medium">Name</label>
              <DInput v-model="newEnvName" placeholder="e.g. DATABASE_URL" :disabled="isAdding" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-neutral-strong text-sm font-medium">Value</label>
              <DInput
                v-model="newEnvValue"
                type="password"
                placeholder="Enter value"
                :disabled="isAdding"
              />
            </div>
          </div>
        </template>
        <template #cancel>
          <d-button variant="secondary">Cancel</d-button>
        </template>
        <template #submit>
          <d-button @click="addEnvVar" :loading="isAdding" :disabled="!newEnvName.trim()">
            Add Variable
          </d-button>
        </template>
      </DDialog>
    </div>

    <!-- Empty state -->
    <div
      v-if="!envVars || envVars.length === 0"
      class="flex flex-col items-center justify-center gap-2 p-12 text-center"
    >
      <div class="text-neutral-moderate text-sm">No environment variables</div>
      <div class="text-neutral-subtle text-xs">
        Environment variables are encrypted and securely stored.
      </div>
    </div>

    <!-- List -->
    <div v-else class="flex-1 overflow-auto">
      <div
        v-for="envVar in envVars"
        :key="envVar.id"
        class="border-neutral-subtle flex items-center justify-between border-b px-4 py-3"
      >
        <div class="flex flex-col gap-1">
          <div class="text-neutral-strong text-sm font-mono">{{ envVar.name }}</div>
          <div class="text-neutral-moderate flex items-center gap-2 text-xs font-mono">
            <span v-if="revealedValues[envVar.id]" class="max-w-md truncate">
              {{ revealedValues[envVar.id] }}
            </span>
            <span v-else class="tracking-wider">********</span>
          </div>
        </div>
        <div class="flex items-center gap-1">
          <d-button
            variant="transparent"
            size="sm"
            @click="revealValue(envVar)"
            :loading="revealingIds.has(envVar.id)"
            :title="revealedValues[envVar.id] ? 'Hide value' : 'Reveal value'"
          >
            <template #leading>
              <EyeOffIcon v-if="revealedValues[envVar.id]" class="size-4" />
              <EyeIcon v-else class="size-4" />
            </template>
          </d-button>

          <!-- Edit Dialog -->
          <DialogRoot v-model:open="showEditDialog">
            <DialogTrigger as-child>
              <d-button
                variant="transparent"
                size="sm"
                @click="openEditDialog(envVar)"
                title="Edit variable"
              >
                <template #leading>
                  <PencilIcon class="size-4" />
                </template>
              </d-button>
            </DialogTrigger>
            <DialogPortal>
              <DialogOverlay class="fixed inset-0 z-30 bg-black/10" />
              <DialogContent
                class="bg-surface border-neutral-subtle fixed top-[50%] left-[50%] z-[100] max-h-[85vh] w-[90vw] max-w-[450px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] border p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
              >
                <DialogTitle class="text-neutral-strong m-0 text-[17px] font-semibold">
                  Edit Environment Variable
                </DialogTitle>
                <DialogDescription class="text-neutral-subtle mt-[10px] mb-5 text-sm leading-normal">
                  Update the name and/or value. Leave value empty to keep the current value.
                </DialogDescription>
                <div class="flex flex-col gap-4">
                  <div class="flex flex-col gap-1">
                    <label class="text-neutral-strong text-sm font-medium">Name</label>
                    <DInput
                      v-model="editEnvName"
                      placeholder="e.g. DATABASE_URL"
                      :disabled="isEditing"
                    />
                  </div>
                  <div class="flex flex-col gap-1">
                    <label class="text-neutral-strong text-sm font-medium">Value</label>
                    <DInput
                      v-model="editEnvValue"
                      type="password"
                      placeholder="Enter new value (leave empty to keep current)"
                      :disabled="isEditing"
                    />
                  </div>
                </div>
                <div class="mt-6 flex justify-between">
                  <DialogClose as-child>
                    <d-button variant="secondary">Cancel</d-button>
                  </DialogClose>
                  <d-button
                    @click="updateEnvVar"
                    :loading="isEditing"
                    :disabled="!editEnvName.trim()"
                  >
                    Save Changes
                  </d-button>
                </div>
              </DialogContent>
            </DialogPortal>
          </DialogRoot>

          <!-- Delete Dialog -->
          <DialogRoot v-model:open="showDeleteDialog">
            <DialogTrigger as-child>
              <d-button
                variant="transparent"
                size="sm"
                @click="openDeleteDialog(envVar)"
                title="Delete variable"
              >
                <template #leading>
                  <TrashIcon class="size-4" />
                </template>
              </d-button>
            </DialogTrigger>
            <DialogPortal>
              <DialogOverlay class="fixed inset-0 z-30 bg-black/10" />
              <DialogContent
                class="bg-surface border-neutral-subtle fixed top-[50%] left-[50%] z-[100] max-h-[85vh] w-[90vw] max-w-[450px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] border p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
              >
                <DialogTitle class="text-neutral-strong m-0 text-[17px] font-semibold">
                  Delete Environment Variable
                </DialogTitle>
                <DialogDescription class="text-neutral-subtle mt-[10px] mb-5 text-sm leading-normal">
                  Are you sure you want to delete
                  <strong class="font-mono">{{ deletingEnvVar?.name }}</strong
                  >? This action cannot be undone.
                </DialogDescription>
                <div class="mt-6 flex justify-between">
                  <DialogClose as-child>
                    <d-button variant="secondary">Cancel</d-button>
                  </DialogClose>
                  <d-button variant="danger" @click="deleteEnvVar" :loading="isDeleting">
                    Delete Variable
                  </d-button>
                </div>
              </DialogContent>
            </DialogPortal>
          </DialogRoot>
        </div>
      </div>
    </div>
  </div>
</template>
