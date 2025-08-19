<script setup lang="ts">
import { ref } from "vue"
import DCombobox from "~/components/d-combobox/index.vue"
import DComboboxItem from "~/components/d-combobox/item.vue"

const selectedValue = ref("")

const frameworks = [
  { value: "vue", label: "Vue.js" },
  { value: "react", label: "React" },
  { value: "angular", label: "Angular" },
  { value: "svelte", label: "Svelte" },
  { value: "nuxt", label: "Nuxt" },
  { value: "next", label: "Next.js" },
  { value: "remix", label: "Remix" },
  { value: "astro", label: "Astro" },
]
</script>

<template>
  <div class="space-y-4 p-8">
    <h2 class="text-lg font-semibold">Combobox Example</h2>

    <DCombobox
      v-model="selectedValue"
      :items="frameworks"
      placeholder="Select a framework"
      search-placeholder="Search frameworks..."
      empty-text="No framework found"
    >
      <DComboboxItem
        v-for="framework in frameworks"
        :key="framework.value"
        :value="framework.value"
        :label="framework.label"
      />
    </DCombobox>

    <p class="text-sm text-gray-600">Selected value: {{ selectedValue || "None" }}</p>
  </div>
</template>
