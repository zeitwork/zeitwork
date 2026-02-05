<script setup lang="ts">
import { MagnifyingGlassIcon, PlusIcon } from "@heroicons/vue/16/solid";

const route = useRoute();
const projectSlug = route.params.project as string;

const { data: domains, refresh: refreshDomains } = await useFetch(
  `/api/projects/${projectSlug}/domains`,
);

const search = ref("");
const domainName = ref("");
const isAddDialogOpen = ref(false);

const setupDomain = ref<(typeof filteredDomains.value)[0] | null>(null);
const isSetupDialogOpen = ref(false);

const editDomain = ref<(typeof filteredDomains.value)[0] | null>(null);
const isEditDialogOpen = ref(false);
const editDomainName = ref("");
const isDeleteConfirmOpen = ref(false);

const filteredDomains = computed(() => {
  if (!domains.value) return [];
  if (!search.value) return domains.value;
  return domains.value.filter((domain) =>
    domain.name.toLowerCase().includes(search.value.toLowerCase()),
  );
});

async function createDomain() {
  if (!domainName.value) return;
  await $fetch(`/api/projects/${projectSlug}/domains`, {
    method: "POST",
    body: { name: domainName.value },
  });
  domainName.value = "";
  isAddDialogOpen.value = false;
  await refreshDomains();
}

async function updateDomain() {
  if (!editDomain.value || !editDomainName.value) return;
  if (editDomainName.value === editDomain.value.name) {
    isEditDialogOpen.value = false;
    return;
  }
  await $fetch(`/api/projects/${projectSlug}/domains/${editDomain.value.id}`, {
    method: "PATCH",
    body: { name: editDomainName.value },
  });
  editDomainName.value = "";
  isEditDialogOpen.value = false;
  await refreshDomains();
}

function handleSetup(domain: (typeof filteredDomains.value)[0]) {
  setupDomain.value = domain;
  isSetupDialogOpen.value = true;
}

function handleEdit(domain: (typeof filteredDomains.value)[0]) {
  editDomain.value = domain;
  editDomainName.value = domain.name;
  isEditDialogOpen.value = true;
}

async function deleteDomain() {
  if (!editDomain.value) return;
  await $fetch(`/api/projects/${projectSlug}/domains/${editDomain.value.id}`, {
    method: "DELETE",
  });
  isDeleteConfirmOpen.value = false;
  isEditDialogOpen.value = false;
  await refreshDomains();
}

function handleDomainVerified() {
  refreshDomains();
}
</script>

<template>
  <div class="flex h-full flex-col">
    <DHeader title="Domains" description="can be assigned to git branches or production.">
      <template #trailing>
        <div class="relative">
          <MagnifyingGlassIcon
            class="text-neutral-weak pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
          />
          <input
            v-model="search"
            type="text"
            placeholder="Search Domains..."
            class="bg-surface-subtle border-neutral text-copy text-neutral placeholder:text-neutral-weak h-8 w-[230px] rounded-md border py-1.5 pr-3 pl-8 outline-none"
          />
        </div>
        <DDialog v-model="isAddDialogOpen">
          <template #trigger>
            <DButton :icon-left="PlusIcon">Add Domain</DButton>
          </template>
          <template #title>Add Domain</template>
          <template #content>
            <form class="flex flex-col gap-3" @submit.prevent="createDomain">
              <div class="flex flex-col gap-1.5">
                <label class="text-copy-sm text-neutral">Domain name</label>
                <DInput v-model="domainName" placeholder="example.com" />
              </div>
            </form>
          </template>
          <template #cancel>
            <DButton variant="secondary">Cancel</DButton>
          </template>
          <template #submit>
            <DButton @click="createDomain">Add Domain</DButton>
          </template>
        </DDialog>
      </template>
    </DHeader>

    <div class="flex-1 overflow-auto">
      <DEmptyState
        v-if="!domains?.length"
        title="No domains yet"
        description="Add a custom domain to make your project accessible on the web."
        action-label="Add Domain"
        :action-icon="PlusIcon"
        @action="isAddDialogOpen = true"
      />
      <template v-else-if="filteredDomains.length">
        <DDomainListItem
          v-for="domain in filteredDomains"
          :key="domain.id"
          :domain="domain"
          :project-slug="projectSlug"
          @setup="handleSetup(domain)"
          @edit="handleEdit(domain)"
          @verified="handleDomainVerified"
        />
      </template>
      <DEmptyState
        v-else
        title="No domains found"
        description="No domains match your search criteria."
      />
    </div>

    <DDomainSetupDialog
      v-if="setupDomain"
      v-model="isSetupDialogOpen"
      :domain="setupDomain"
      :project-slug="projectSlug"
      @verified="handleDomainVerified"
    />

    <DDialog v-model="isEditDialogOpen">
      <template #title>Edit Domain</template>
      <template #content>
        <form class="flex flex-col gap-3" @submit.prevent="updateDomain">
          <div class="flex flex-col gap-1.5">
            <label class="text-copy-sm text-neutral">Domain name</label>
            <DInput v-model="editDomainName" placeholder="example.com" />
          </div>
        </form>
      </template>
      <template #footer-left>
        <DAlertDialog v-model="isDeleteConfirmOpen">
          <template #trigger>
            <DButton variant="danger-light" size="sm">Delete</DButton>
          </template>
          <template #title>Delete domain</template>
          <template #content>
            <p class="text-copy text-neutral-subtle">
              Are you sure you want to delete
              <strong class="text-neutral">{{ editDomain?.name }}</strong>?
              This will remove the domain and its DNS configuration.
            </p>
          </template>
          <template #cancel>
            <DButton variant="secondary">Cancel</DButton>
          </template>
          <template #action>
            <DButton variant="danger" @click="deleteDomain">Delete</DButton>
          </template>
        </DAlertDialog>
      </template>
      <template #cancel>
        <DButton variant="secondary">Cancel</DButton>
      </template>
      <template #submit>
        <DButton @click="updateDomain">Save</DButton>
      </template>
    </DDialog>
  </div>
</template>
