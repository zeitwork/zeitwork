<script setup lang="ts">
import {
  MagnifyingGlassIcon,
  PlusIcon,
  XMarkIcon,
} from "@heroicons/vue/16/solid";

definePageMeta({
  layout: "modal",
});

const route = useRoute();
const org = route.params.org;

const { data: connections } = await useFetch(`/api/github/organisations`);

const githubConnections = computed(() => {
  if (!connections.value) return [];
  return connections.value.map((connection) => ({
    value: connection.account,
    label: connection.account,
  }));
});

const selectedValue = ref(githubConnections.value[0]?.value);

const { data: projectList } = useFetch(`/api/github/repositories`, {
  params: {
    account: selectedValue,
  },
});

const search = ref("");

const projects = computed(() => {
  if (!projectList.value) return [];
  return projectList.value.map((project) => ({
    value: project.fullName,
    label: project.fullName,
    framework: "go",
  }));
});

const filteredProjects = computed(() => {
  if (!projects.value) return [];
  return projects.value.filter((project) =>
    project.label.toLowerCase().includes(search.value.toLowerCase()),
  );
});

const config = useRuntimeConfig();

// GitHub App installation URL
const githubAppInstallUrl = computed(() => {
  const appName = config.public.appName;
  const redirectUri = `${config.public.appUrl}/auth/github`;
  return `https://github.com/apps/${appName}/installations/new?redirect_uri=${redirectUri}`;
});

function addGitHubAccount() {
  window.location.href = githubAppInstallUrl.value;
}
</script>

<template>
  <div class="p-5">
    <DButton variant="secondary" :to="`/${org}`" :icon-left="XMarkIcon" />

    <div class="mx-auto mt-20 flex max-w-xl flex-col gap-4">
      <h1 class="text-title-sm text-primary">Import from Git</h1>
      <div class="bg-surface-1 w-full rounded-[14px] p-0.5">
        <div class="bg-surface-0 flex flex-col gap-2 rounded-xl p-2">
          <div class="flex items-center gap-2">
            <DCombobox
              class="w-50"
              v-model="selectedValue"
              :items="githubConnections"
              placeholder="Select an account"
              search-placeholder="Search accounts..."
              empty-text="No framework found"
            >
              <template #trigger="{ selectedItem, selectedLabel, placeholder }">
                <div class="flex items-center gap-2">
                  <img src="/icons/github-mark.svg" />
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
                  <template #leading
                    ><img src="/icons/github-mark.svg"
                  /></template>
                </DComboboxItem>
              </template>
              <template #footer>
                <div
                  class="border-edge-subtle mt-1 border-t pt-1"
                  @click.prevent="addGitHubAccount"
                >
                  <DComboboxItem value="add-account" label="Add GitHub account">
                    <template #leading>
                      <PlusIcon class="size-4" />
                    </template>
                  </DComboboxItem>
                </div>
              </template>
            </DCombobox>
            <DInput
              v-model="search"
              class="flex-1"
              :leading-background="false"
              type="text"
              placeholder="Search projects..."
            >
              <template #leading>
                <MagnifyingGlassIcon class="size-4" />
              </template>
            </DInput>
          </div>
          <div class="flex flex-col gap-1">
            <DHover v-for="project in filteredProjects" class="group w-full">
              <NuxtLink
                class="flex h-10 w-full items-center justify-between p-2 pr-[6px]"
                :to="`/${org}/new/setup`"
              >
                <div class="flex items-center gap-2">
                  <div
                    class="bg-surface-1 size-6 overflow-hidden rounded-full"
                  >
                    <img
                      :src="`/icons/framework/${project.framework}.png`"
                      class="size-full"
                      alt=""
                    />
                  </div>
                  <p class="text-primary text-copy">{{ project.label }}</p>
                </div>
                <DButton
                  variant="primary"
                  :to="`/${org}/new/setup?repo=${project.value}`"
                  >Import</DButton
                >
              </NuxtLink>
            </DHover>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
