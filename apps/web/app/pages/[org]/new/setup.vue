<script setup lang="ts">
import { GitMergeIcon } from "lucide-vue-next";
import { PlusIcon, XMarkIcon, ArrowLeftIcon } from "@heroicons/vue/16/solid";

definePageMeta({
  layout: "modal",
});

const route = useRoute();
const repo = route.query.repo as string;

const selectedTeam = ref<string | undefined>("dokedu");
const teams = [
  {
    value: "aaronmahlke",
    label: "Aaron Mahlke",
  },
  {
    value: "dokedu",
    label: "Dokedu",
  },
];

const selectedFramework = ref<string | undefined>("nuxt");
const frameworks = [
  {
    value: "nuxt",
    label: "Nuxt",
  },
  {
    value: "next",
    label: "Next.js",
  },
  {
    value: "rails",
    label: "Ruby on Rails",
  },
  {
    value: "go",
    label: "Go",
  },
];

const projectName = ref<string | undefined>("");
const rootDirectory = ref<string | undefined>("");

const org = route.params.org;

interface EnvVariable {
  name: string;
  value: string;
}

const envVariables = ref<EnvVariable[]>([{ name: "", value: "" }]);
const errorMessage = ref<string | null>(null);

function addEnvVariable() {
  envVariables.value.push({ name: "", value: "" });
}

function removeEnvVariable(index: number) {
  // only remove if not the last one
  if (envVariables.value.length === 1) return;
  envVariables.value.splice(index, 1);
}

const repoOwner = computed(() => repo?.split("/")[0]);
const repoName = computed(() => repo?.split("/")[1]);

async function createProject() {
  errorMessage.value = null;
  try {
    // Normalize rootDirectory: empty string or just "/" stays as "/", otherwise prepend "/" if missing
    let normalizedRootDir = rootDirectory.value?.trim() || "/";
    if (normalizedRootDir !== "/" && !normalizedRootDir.startsWith("/")) {
      normalizedRootDir = "/" + normalizedRootDir;
    }

    const result = await $fetch(`/api/projects`, {
      method: "POST",
      body: {
        name: projectName.value,
        repository: {
          owner: repoOwner.value,
          repo: repoName.value,
        },
        secrets: envVariables.value.filter((e) => e.name.length > 0),
        rootDirectory: normalizedRootDir,
      },
    });
    navigateTo(`/${org}/${projectName.value}`);
  } catch (error: any) {
    console.error("Failed to create project:", error);
    if (error?.statusCode === 403) {
      errorMessage.value = error?.data?.message || error?.message || "Project limit reached";
    } else {
      errorMessage.value = "Failed to create project. Please try again.";
    }
  }
}
</script>

<template>
  <div class="p-5">
    <div>
      <DButton variant="secondary" :to="`/${org}`" :icon-left="XMarkIcon" />
    </div>

    <div class="mx-auto mt-20 flex max-w-xl flex-col gap-4">
      <div>
        <h1 class="text-title-sm text-primary">New project</h1>
        <p class="text-secondary text-copy-sm mt-2">
          We require your project to have a <code class="text-primary rounded bg-surface-2 px-1">Dockerfile</code> and expose on port <code class="text-primary rounded bg-surface-2 px-1">3000</code>.
          Feel free to DM <a href="https://x.com/tomhaerter" target="_blank" class="underline hover:no-underline">x.com/tomhaerter</a> if you have other requirements.
          We're working on automatic framework support - <a href="https://github.com/zeitwork/zeitwork/issues/15" target="_blank" class="underline hover:no-underline">upvote the issue</a> if you'd like to see it sooner.
        </p>
      </div>
      <div class="bg-surface-2 w-full rounded-[14px] p-0.5">
        <div class="flex flex-col gap-1 px-[18px] py-3">
          <p class="text-secondary text-copy-sm">Importing from GitHub</p>
          <div class="text-primary text-copy flex items-center gap-1">
            <img src="/icons/github-mark.svg" />
            <p>
              <span>{{ repo }}</span>
            </p>
            <p class="text-secondary flex items-center gap-1">
              <GitMergeIcon class="size-3" stroke-width="2" />
              <span>main</span>
            </p>
          </div>
        </div>
        <div class="base-3 flex flex-col gap-2 rounded-xl">
          <div class="flex flex-col gap-3 p-[18px]">
            <div class="flex w-full items-end gap-2">
              <DFormGroup v-if="false" class="flex-1">
                <DLabel class="text-primary text-copy-sm">Team</DLabel>
                <DCombobox
                  v-model="selectedTeam"
                  :items="teams"
                  placeholder="Select a team"
                  search-placeholder="Search teams..."
                  empty-text="No teams found"
                >
                  <template #trigger="{ selectedItem, selectedLabel, placeholder }">
                    <div class="flex items-center gap-2">
                      <img
                        v-if="selectedItem"
                        :src="`/icons/team/${selectedItem?.value}.png`"
                        class="size-5"
                      />
                      <div v-else class="bg-surface-2 size-5 rounded-full"></div>
                      {{ selectedItem ? selectedItem.label : placeholder }}
                    </div>
                  </template>
                  <template #default="{ filteredItems }">
                    <DComboboxItem
                      v-for="item in filteredItems"
                      :key="item.value"
                      :value="item.value"
                      :label="item.label"
                    >
                      <template #leading>
                        <img :src="`/icons/team/${item.value}.png`" class="size-5" />
                      </template>
                    </DComboboxItem>
                  </template>
                  <template #footer>
                    <div class="border-edge-subtle mt-1 border-t pt-1">
                      <DComboboxItem value="add-account" label="Create a Team">
                        <template #leading>
                          <PlusIcon class="size-4" />
                        </template>
                      </DComboboxItem>
                    </div>
                  </template>
                </DCombobox>
              </DFormGroup>
              <DFormGroup class="flex-1">
                <DLabel class="text-primary text-copy-sm">Project Name</DLabel>
                <DInput type="text" placeholder="Enter project name..." v-model="projectName" />
              </DFormGroup>
            </div>
            <DFormGroup>
              <DLabel>Root Directory</DLabel>
              <DInput type="text" placeholder="Leave blank for root" v-model="rootDirectory" />
            </DFormGroup>
            <DFormGroup v-if="false" class="flex-1">
              <DLabel class="text-primary text-copy-sm">Framework</DLabel>
              <DCombobox
                v-model="selectedFramework"
                :items="frameworks"
                placeholder="Select your framework"
                search-placeholder="Search frameworks..."
                empty-text="No frameworks found"
              >
                <template #trigger="{ selectedItem, selectedLabel, placeholder }">
                  <div class="flex items-center gap-2">
                    <img
                      v-if="selectedItem"
                      :src="`/icons/framework/${selectedItem?.value}.png`"
                      class="size-5"
                    />
                    <div v-else class="bg-surface-2 size-5 rounded-full"></div>
                    {{ selectedItem ? selectedItem.label : placeholder }}
                  </div>
                </template>
                <template #default="{ filteredItems }">
                  <DComboboxItem
                    v-for="item in filteredItems"
                    :key="item.value"
                    :value="item.value"
                    :label="item.label"
                  >
                    <template #leading>
                      <img :src="`/icons/framework/${item.value}.png`" class="size-5" />
                    </template>
                  </DComboboxItem>
                </template>
                <template #footer>
                  <div class="border-edge-subtle mt-1 border-t pt-1">
                    <DComboboxItem value="add-account" label="Add Dockerfile">
                      <template #leading>
                        <PlusIcon class="size-4" />
                      </template>
                    </DComboboxItem>
                  </div>
                </template>
              </DCombobox>
            </DFormGroup>
            <div class="flex flex-col gap-2">
              <DLabel class="text-primary text-copy-sm">Environment Variables</DLabel>
              <div class="flex flex-col gap-2">
                <div
                  v-for="(envVariable, index) in envVariables"
                  :key="index"
                  class="grid grid-cols-[1fr_2fr_auto] gap-2"
                >
                  <DInput
                    type="text"
                    placeholder="Name"
                    class="w-full"
                    v-model="envVariable.name"
                  />
                  <DInput
                    type="text"
                    placeholder="Value"
                    class="w-full"
                    v-model="envVariable.value"
                  />
                  <DButton
                    variant="secondary"
                    :icon-left="XMarkIcon"
                    @click="removeEnvVariable(index)"
                  />
                </div>
                <div>
                  <DButton variant="secondary" :icon-left="PlusIcon" @click="addEnvVariable"
                    >Add</DButton
                  >
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center justify-between px-[18px] py-3">
          <DButton variant="transparent" :icon-left="ArrowLeftIcon" :to="`/${org}/new`"
            >Back</DButton
          >
          <div class="flex flex-col items-end gap-2">
            <p v-if="errorMessage" class="text-copy-sm text-danger">{{ errorMessage }}</p>
            <DButton variant="primary" @click="createProject">Deploy</DButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
