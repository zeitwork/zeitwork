<script setup lang="ts">
const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);
const projectName = computed<string>(() => route.params.project as string);
const environmentName = computed<string>(() => route.params.env as string);
</script>
<template>
  <div class="flex items-center px-3 py-2">
    <NuxtLink :to="`/${orgSlug}`" class="grid size-8 place-items-center">
      <d-logo class="text-neutral-strong size-4" />
    </NuxtLink>
    <d-page-header-separator />
    <d-page-header-breadcrumb-link :to="`/${orgSlug}`" :name="orgSlug" />
    <template v-if="projectName">
      <DPageHeaderSeparator />
      <DPageHeaderBreadcrumbLink :name="projectName as string" :to="`/${orgSlug}/${projectName}`" />
    </template>
    <template v-if="environmentName">
      <DPageHeaderSeparator />
      <DPageHeaderBreadcrumbLink
        :name="environmentName as string"
        :to="`/${orgSlug}/${projectName}/${environmentName}`"
      />
    </template>
  </div>
</template>
