<script setup lang="ts">
const route = useRoute();
const orgId = route.params.org as string;
const projectSlug = route.params.project as string;

const { data: domains, refresh: refreshDomains } = await useFetch(
  `/api/projects/${projectSlug}/domains`,
);

const domainName = ref("");

async function createDomain() {
  try {
    if (!domainName.value) return;
    await $fetch(`/api/projects/${projectSlug}/domains`, {
      method: "POST",
      body: { name: domainName.value },
    });
    domainName.value = "";
    await refreshDomains();
  } catch (error) {
    console.error("Failed to create domain:", error);
  }
}
</script>

<template>
  <div class="flex flex-col">
    <div class="border-neutral-subtle flex items-center justify-between border-b p-4">
      <div class="text-neutral-strong text-sm">Domains</div>
      <DDialog>
        <template #trigger>
          <d-button>Create Domain</d-button>
        </template>
        <template #title>Create Domain</template>
        <template #description>Create a new domain</template>
        <template #content>
          <div class="flex flex-col gap-2">
            <DInput v-model="domainName" placeholder="Name of your domain" />
          </div>
        </template>
        <template #cancel>
          <d-button>Close</d-button>
        </template>
        <template #submit>
          <d-button @click="createDomain">Create</d-button>
        </template>
      </DDialog>
    </div>
    <div class="flex-1 overflow-auto">
      <div
        v-for="domain in domains"
        :key="domain.id"
        class="border-neutral-subtle flex justify-between border-b p-4"
      >
        <div class="text-neutral text-sm">{{ domain.name }}</div>
        <div class="text-neutral text-sm">{{ domain.verifiedAt ? "Verified" : "Unverified" }}</div>
      </div>
    </div>
  </div>
</template>
