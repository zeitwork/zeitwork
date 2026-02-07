<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
const orgId = route.params.org as string;
const projectSlug = route.params.project as string;
</script>

<template>
  <div class="flex h-full flex-col overflow-auto">
    <div class="flex-1 overflow-auto">
      <div class="flex h-full shrink-0 flex-row overflow-auto">
        <div class="border-edge flex w-64 flex-col gap-1 border-r p-2">
          <DNavbarLink
            name="General"
            :active="route.path == `/${orgId}/${projectSlug}/settings`"
            :to="`/${orgId}/${projectSlug}/settings`"
            full-width
          />
          <DNavbarLink
            name="Domains"
            :active="route.path == `/${orgId}/${projectSlug}/settings/domains`"
            :to="`/${orgId}/${projectSlug}/settings/domains`"
            full-width
          />
          <DNavbarLink
            name="Environment Variables"
            :active="
              route.path ==
              `/${orgId}/${projectSlug}/settings/environment-variables`
            "
            :to="`/${orgId}/${projectSlug}/settings/environment-variables`"
            full-width
          />
        </div>
        <div class="flex flex-1 flex-col overflow-auto">
          <div class="flex-1 overflow-auto">
            <NuxtPage />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
