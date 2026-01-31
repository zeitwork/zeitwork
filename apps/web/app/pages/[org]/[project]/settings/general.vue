<script setup lang="ts">
const route = useRoute();
const orgId = route.params.org as string;
const projectSlug = route.params.project as string;
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
    </div>
  </div>
</template>
