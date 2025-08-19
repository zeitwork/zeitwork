<script setup lang="ts">
import { MagnifyingGlassIcon, PlusIcon } from "@heroicons/vue/16/solid"

definePageMeta({
  layout: "modal",
})

const route = useRoute()
const org = route.params.org

const selectedValue = ref("aaronmahlke")

const githubConnections = [
  { value: "aaronmahlke", label: "aaronmahlke" },
  { value: "dokedu", label: "dokedu" },
  { value: "zeitwork", label: "zeitwork" },
  { value: "analog", label: "analog" },
  { value: "opencut", label: "opencut" },
]

const projects = [
  { value: "dokedu", label: "dokedu", framework: "nuxt" },
  { value: "zeitwork", label: "zeitwork", framework: "go" },
  { value: "help-me-write", label: "help-me-write", framework: "next" },
  { value: "momentso", label: "momentso", framework: "rails" },
  { value: "meilso", label: "meilso", framework: "nuxt" },
]
</script>
<template>
  <div class="p-5">
    <DButton variant="secondary" :to="`/${org}`">Back</DButton>

    <div class="bg-surface-subtle mx-auto mt-20 max-w-xl rounded-[14px] p-0.5">
      <div class="bg-surface flex flex-col gap-2 rounded-xl p-2">
        <div class="flex items-center gap-2">
          <DCombobox
            class="w-50"
            v-model="selectedValue"
            :items="githubConnections"
            placeholder="Select an account"
            search-placeholder="Search accounts..."
            empty-text="No framework found"
          >
            <template #trigger="{ selectedItem, selectedLabel, placeholder }">
              <div class="flex items-center gap-2">
                <img src="/icons/github-mark.svg" />
                {{ selectedItem ? selectedItem.label : placeholder }}
              </div>
            </template>
            <template #default="{ filteredItems }">
              <DComboboxItem v-for="item in filteredItems" :key="item.value" :value="item.value" :label="item.label">
                <template #leading><img src="/icons/github-mark.svg" /></template>
              </DComboboxItem>
            </template>
            <template #footer>
              <div class="border-neutral-subtle mt-1 border-t pt-1">
                <DComboboxItem value="add-account" label="Add GitHub account">
                  <template #leading>
                    <PlusIcon class="size-4" />
                  </template>
                </DComboboxItem>
              </div>
            </template>
          </DCombobox>
          <DInput class="flex-1" :leading-background="false" type="text" placeholder="Search projects...">
            <template #leading> <MagnifyingGlassIcon class="size-4" /> </template>
          </DInput>
        </div>
        <div class="flex flex-col gap-1">
          <DHover v-for="project in projects" class="group">
            <NuxtLink class="flex h-10 w-full items-center justify-between p-2 pr-[6px]" :to="`/${org}/new/setup`">
              <div class="flex items-center gap-2">
                <div class="bg-surface-subtle size-6 overflow-hidden rounded-full">
                  <img :src="`/icons/framework/${project.framework}.png`" class="size-full" alt="" />
                </div>
                <p class="text-neutral text-copy">{{ project.label }}</p>
              </div>
              <DButton variant="primary" :to="`/${org}/new/setup?framework=${project.framework}`">Import</DButton>
            </NuxtLink>
          </DHover>
        </div>
      </div>
    </div>
  </div>
</template>
