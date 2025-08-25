<script setup lang="ts">
import { Motion, AnimatePresence } from "motion-v"

import { GitMergeIcon } from "lucide-vue-next"
import { PlusIcon, XMarkIcon, ArrowLeftIcon } from "@heroicons/vue/16/solid"

definePageMeta({
  layout: "modal",
})

const selectedTeam = ref<string | undefined>("dokedu")
const teams = [
  {
    value: "aaronmahlke",
    label: "Aaron Mahlke",
  },
  {
    value: "dokedu",
    label: "Dokedu",
  },
]

const selectedFramework = ref<string | undefined>("nuxt")
const frameworks = [
  {
    value: "nuxt",
    label: "Nuxt",
  },
  {
    value: "next",
    label: "Next.js",
  },
  {
    value: "rails",
    label: "Ruby on Rails",
  },
  {
    value: "go",
    label: "Go",
  },
]

const projectName = ref<string | undefined>("dokedu")
const rootDirectory = ref<string | undefined>("./")

const route = useRoute()
const org = route.params.org
</script>
<template>
  <MotionConfig
    :transition="{
      type: 'spring',
      stiffness: 400,
      damping: 30,
    }"
  >
    <Motion class="p-5" layoutId="page-container">
      <Motion layoutId="back-button">
        <DButton variant="secondary" :to="`/${org}`" :icon-left="XMarkIcon" />
      </Motion>

      <Motion class="mx-auto mt-20 flex max-w-xl flex-col gap-4" layoutId="main-content">
        <AnimatePresence mode="wait">
          <Motion
            key="setup-title"
            :initial="{ opacity: 0, y: 20 }"
            :animate="{ opacity: 1, y: 0 }"
            :exit="{ opacity: 0, y: -20 }"
          >
            <h1 class="text-title-sm text-neutral">New project</h1>
          </Motion>
        </AnimatePresence>
        <Motion class="bg-surface-subtle w-full rounded-[14px] p-0.5" layoutId="content-container">
          <div class="flex flex-col gap-1 px-[18px] py-3">
            <p class="text-neutral-subtle text-copy-sm">Importing from GitHub</p>
            <div class="text-neutral text-copy flex items-center gap-1">
              <img src="/icons/github-mark.svg" />
              <p>
                <span>dokedu</span>
                <span>/</span>
                <span>dokedu</span>
              </p>
              <p class="text-neutral-subtle flex items-center gap-1">
                <GitMergeIcon class="size-3" stroke-width="2" />
                <span>main</span>
              </p>
            </div>
          </div>
          <AnimatePresence mode="wait">
            <Motion key="setup-content" class="bg-surface flex flex-col gap-2 rounded-xl" layoutId="content-inner">
              <div class="flex flex-col gap-3 p-[18px]">
                <div class="flex w-full items-end gap-2">
                  <DFormGroup class="flex-1">
                    <DLabel class="text-neutral text-copy-sm">Team</DLabel>
                    <DCombobox
                      v-model="selectedTeam"
                      :items="teams"
                      placeholder="Select a team"
                      search-placeholder="Search teams..."
                      empty-text="No teams found"
                    >
                      <template #trigger="{ selectedItem, selectedLabel, placeholder }">
                        <div class="flex items-center gap-2">
                          <img v-if="selectedItem" :src="`/icons/team/${selectedItem?.value}.png`" class="size-5" />
                          <div v-else class="bg-surface-strong size-5 rounded-full"></div>
                          {{ selectedItem ? selectedItem.label : placeholder }}
                        </div>
                      </template>
                      <template #default="{ filteredItems }">
                        <DComboboxItem
                          v-for="item in filteredItems"
                          :key="item.value"
                          :value="item.value"
                          :label="item.label"
                        >
                          <template #leading>
                            <img :src="`/icons/team/${item.value}.png`" class="size-5" />
                          </template>
                        </DComboboxItem>
                      </template>
                      <template #footer>
                        <div class="border-neutral-subtle mt-1 border-t pt-1">
                          <DComboboxItem value="add-account" label="Create a Team">
                            <template #leading>
                              <PlusIcon class="size-4" />
                            </template>
                          </DComboboxItem>
                        </div>
                      </template>
                    </DCombobox>
                  </DFormGroup>
                  <DFormGroup class="flex-1">
                    <DLabel class="text-neutral text-copy-sm">Project Name</DLabel>
                    <DInput type="text" placeholder="Enter project name..." v-model="projectName" />
                  </DFormGroup>
                </div>
                <DFormGroup>
                  <DLabel>Root Directory</DLabel>
                  <DInput type="text" class="font-mono" placeholder="Enter root directory..." v-model="rootDirectory" />
                </DFormGroup>
                <DFormGroup class="flex-1">
                  <DLabel class="text-neutral text-copy-sm">Framework</DLabel>
                  <DCombobox
                    v-model="selectedFramework"
                    :items="frameworks"
                    placeholder="Select your framework"
                    search-placeholder="Search frameworks..."
                    empty-text="No frameworks found"
                  >
                    <template #trigger="{ selectedItem, selectedLabel, placeholder }">
                      <div class="flex items-center gap-2">
                        <img v-if="selectedItem" :src="`/icons/framework/${selectedItem?.value}.png`" class="size-5" />
                        <div v-else class="bg-surface-strong size-5 rounded-full"></div>
                        {{ selectedItem ? selectedItem.label : placeholder }}
                      </div>
                    </template>
                    <template #default="{ filteredItems }">
                      <DComboboxItem
                        v-for="item in filteredItems"
                        :key="item.value"
                        :value="item.value"
                        :label="item.label"
                      >
                        <template #leading>
                          <img :src="`/icons/framework/${item.value}.png`" class="size-5" />
                        </template>
                      </DComboboxItem>
                    </template>
                    <template #footer>
                      <div class="border-neutral-subtle mt-1 border-t pt-1">
                        <DComboboxItem value="add-account" label="Add Dockerfile">
                          <template #leading>
                            <PlusIcon class="size-4" />
                          </template>
                        </DComboboxItem>
                      </div>
                    </template>
                  </DCombobox>
                </DFormGroup>
              </div>
            </Motion>
          </AnimatePresence>
          <div class="flex items-center justify-between px-[18px] py-3">
            <DButton variant="transparent" :icon-left="ArrowLeftIcon" :to="`/${org}/new`">Back</DButton>
            <DButton variant="primary" :to="`/${org}/something/`">Deploy</DButton>
          </div>
        </Motion>
      </Motion>
    </Motion>
  </MotionConfig>
</template>
