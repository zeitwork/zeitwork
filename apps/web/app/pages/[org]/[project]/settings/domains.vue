<script setup lang="ts">
import { MagnifyingGlassIcon, PlusIcon } from "@heroicons/vue/16/solid";

const route = useRoute();
const projectSlug = route.params.project as string;

const { data: domains, refresh: refreshDomains } = await useFetch(
  `/api/projects/${projectSlug}/domains`,
);

type Domain = NonNullable<typeof domains.value>[number];

const search = ref("");
const domainName = ref("");
const isAddDialogOpen = ref(false);

const setupDomain = ref<Domain>();
const isSetupDialogOpen = ref(false);

const editDomain = ref<Domain>();
const isEditDialogOpen = ref(false);
const editDomainName = ref("");
const isRedirectEnabled = ref(false);
const editRedirectTo = ref("");
const editRedirectStatusCode = ref("301");
const isDeleteConfirmOpen = ref(false);

const redirectOptions = [
  { value: "301", display: "301 Moved Permanently" },
  { value: "302", display: "302 Found" },
  { value: "307", display: "307 Temporary Redirect" },
  { value: "308", display: "308 Permanent Redirect" },
];

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
  
  const newRedirectTo = isRedirectEnabled.value && editRedirectTo.value ? editRedirectTo.value : null;
  const newRedirectStatusCode = isRedirectEnabled.value && editRedirectTo.value ? parseInt(editRedirectStatusCode.value) : null;
  
  if (editDomainName.value === editDomain.value.name && 
      newRedirectTo === (editDomain.value.redirectTo || null) &&
      newRedirectStatusCode === (editDomain.value.redirectStatusCode || null)) {
    isEditDialogOpen.value = false;
    return;
  }
  
  const payload: any = { name: editDomainName.value };
  payload.redirectTo = newRedirectTo;
  payload.redirectStatusCode = newRedirectStatusCode;
  
  await $fetch(`/api/projects/${projectSlug}/domains/${editDomain.value.id}`, {
    method: "PATCH",
    body: payload,
  });
  editDomainName.value = "";
  isEditDialogOpen.value = false;
  await refreshDomains();
}

function handleSetup(domain: Domain) {
  setupDomain.value = domain;
  isSetupDialogOpen.value = true;
}

function handleEdit(domain: Domain) {
  editDomain.value = domain;
  editDomainName.value = domain.name;
  isRedirectEnabled.value = !!domain.redirectTo;
  editRedirectTo.value = domain.redirectTo || "";
  editRedirectStatusCode.value = String(domain.redirectStatusCode || 301);
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
            class="text-tertiary pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
          />
          <input
            v-model="search"
            type="text"
            placeholder="Search Domains..."
            class="bg-surface-1 border-edge text-copy text-primary placeholder:text-tertiary h-8 w-[230px] rounded-md border py-1.5 pr-3 pl-8 outline-none"
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
                <label class="text-copy-sm text-primary">Domain name</label>
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
        <form class="flex flex-col gap-6" @submit.prevent="updateDomain">
          <div class="flex flex-col gap-1.5">
            <label class="text-copy-sm text-primary">Domain name</label>
            <DInput v-model="editDomainName" placeholder="example.com" />
          </div>

          <div class="border-edge-subtle border-t pt-5 flex flex-col gap-4">
            <div class="flex items-center justify-between">
              <div>
                <h4 class="text-copy-sm text-primary font-medium">Domain Redirect</h4>
                <p class="text-secondary text-sm">Redirect traffic from this domain to another URL.</p>
              </div>
              <DSwitch v-model="isRedirectEnabled" />
            </div>

            <div v-if="isRedirectEnabled" class="flex flex-col gap-3 rounded-lg border border-edge bg-surface-1/50 p-4">
              <div class="flex flex-col gap-1.5">
                <label class="text-copy-sm text-primary">Destination URL</label>
                <DInput v-model="editRedirectTo" placeholder="https://example.com" />
              </div>
              <div class="flex flex-col gap-1.5">
                <label class="text-copy-sm text-primary">Status Code</label>
                <DSelect v-model="editRedirectStatusCode" :options="redirectOptions" />
              </div>
            </div>
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
            <p class="text-copy text-secondary">
              Are you sure you want to delete
              <strong class="text-primary">{{ editDomain?.name }}</strong>?
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
