<script setup lang="ts">
import { PlusIcon } from "@heroicons/vue/16/solid"
</script>
<template>
  <div class="flex">
    <div class="flex flex-col gap-2 p-5">
      <div class="flex gap-2">
        <DButton size="xs">Create Page</DButton>
        <DButton size="xs" :icon-left="PlusIcon">Create Page</DButton>
        <DButton size="xs" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton size="sm">Create Page</DButton>
        <DButton size="sm" :icon-left="PlusIcon">Create Page</DButton>
        <DButton size="sm" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton size="md">Create Page</DButton>
        <DButton size="md" :icon-left="PlusIcon">Create Page</DButton>
        <DButton size="md" :icon-left="PlusIcon" />
      </div>
    </div>
    <div class="flex flex-col gap-2 p-5">
      <div class="flex gap-2">
        <DButton variant="secondary" size="xs">Create Page</DButton>
        <DButton variant="secondary" size="xs" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="secondary" size="xs" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="secondary" size="sm">Create Page</DButton>
        <DButton variant="secondary" size="sm" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="secondary" size="sm" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="secondary" size="md">Create Page</DButton>
        <DButton variant="secondary" size="md" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="secondary" size="md" :icon-left="PlusIcon" />
      </div>
    </div>
    <div class="flex flex-col gap-2 p-5">
      <div class="flex gap-2">
        <DButton variant="transparent" size="xs">Create Page</DButton>
        <DButton variant="transparent" size="xs" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="transparent" size="xs" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="transparent" size="sm">Create Page</DButton>
        <DButton variant="transparent" size="sm" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="transparent" size="sm" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="transparent" size="md">Create Page</DButton>
        <DButton variant="transparent" size="md" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="transparent" size="md" :icon-left="PlusIcon" />
      </div>
    </div>
    <div class="flex flex-col gap-2 p-5">
      <div class="flex gap-2">
        <DButton variant="danger" size="xs">Create Page</DButton>
        <DButton variant="danger" size="xs" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger" size="xs" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="danger" size="sm">Create Page</DButton>
        <DButton variant="danger" size="sm" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger" size="sm" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="danger" size="md">Create Page</DButton>
        <DButton variant="danger" size="md" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger" size="md" :icon-left="PlusIcon" />
      </div>
    </div>
    <div class="flex flex-col gap-2 p-5">
      <div class="flex gap-2">
        <DButton variant="danger-light" size="xs">Create Page</DButton>
        <DButton variant="danger-light" size="xs" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger-light" size="xs" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="danger-light" size="sm">Create Page</DButton>
        <DButton variant="danger-light" size="sm" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger-light" size="sm" :icon-left="PlusIcon" />
      </div>
      <div class="flex gap-2">
        <DButton variant="danger-light" size="md">Create Page</DButton>
        <DButton variant="danger-light" size="md" :icon-left="PlusIcon">Create Page</DButton>
        <DButton variant="danger-light" size="md" :icon-left="PlusIcon" />
      </div>
    </div>
  </div>
</template>
