<script setup lang="ts">
const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);

interface Link {
  name: string;
  to: string;
  active?: boolean;
}

interface Props {
  links: Link[];
}

const { links } = defineProps<Props>();
</script>

<template>
  <div class="flex gap-2 p-2">
    <d-navbar-link
      v-for="link in links"
      :key="link.name"
      :to="link.to"
      :name="link.name"
      :active="link.active"
    />
  </div>
</template>
