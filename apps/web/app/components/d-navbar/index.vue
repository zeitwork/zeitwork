<script setup lang="ts">
import Component from "#components";

const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);

type Link = {
  name: string;
  icon?: Component;
  to: string;
  active?: boolean;
};

type Props = {
  links: Link[];
};

const { links } = defineProps<Props>();
</script>

<template>
  <div class="flex gap-2 p-2">
    <DNavbarLink
      v-for="link in links"
      :key="link.name"
      :to="link.to"
      :name="link.name"
      :icon="link.icon"
      :active="link.active"
    />
  </div>
</template>
