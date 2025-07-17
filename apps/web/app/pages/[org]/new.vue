<script setup lang="ts">
const route = useRoute()
const orgId = computed<string>(() => route.params.org as string)
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <div class="flex gap-2">
        <DInput class="flex-1" placeholder="Search Projects..." />
        <DButton :to="`/${orgId}/new`">Add Project</DButton>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <pre>{{ "hello, world" }}</pre>
      </div>
    </div>
  </DPageWrapper>
</template>
