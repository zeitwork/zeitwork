<script setup lang="ts">
import Component from "#components";

const route = useRoute();
const orgSlug = computed<string>(() => route.params.org as string);
const projectSlug = computed<string>(() => route.params.project as string);

type Props = {
  to: string;
  name: string;
  icon?: Component;
  active?: boolean;
  fullWidth?: boolean;
};

const { to, name, icon, active, fullWidth = false } = defineProps<Props>();
</script>

<template>
  <DHover :active="active" background="bg-surface-strong" :full-width="fullWidth">
    <NuxtLink
      class="flex h-8 items-center px-2 text-copy gap-0.5"
      :class="[
        active ? 'text-neutral' : 'text-neutral-subtle group-hover/h:text-neutral',
        fullWidth ? 'w-full' : '',
      ]"
      :to="to"
    >
      <div v-if="icon" class="size-5 grid place-items-center">
        <component :is="icon" class="text-neutral-subtle size-4" />
      </div>
      <div class="px-0.5">
        {{ name }}
      </div>
    </NuxtLink>
  </DHover>
</template>
