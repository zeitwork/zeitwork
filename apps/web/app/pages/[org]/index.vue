<script setup lang="ts">
type Project = {
  name: string
  domain: string
  githubUrl: string
  image: string
  commitMessage: string
  date: string
  branch: string
}
const projects = ref<Project[]>([
  {
    name: "klarbook",
    domain: "app.klarbook.com",
    githubUrl: "klarbook/klarbook",
    image: "https://example.com/project-image.jpg",
    commitMessage: "Enhance document filtering logic: Improve null and suggestion",
    date: "2025-07-13T17:09:26Z",
    branch: "main",
  },
  {
    name: "zeitwork",
    domain: "app.zeitwork.com",
    githubUrl: "zeitwork/zeitwork",
    image: "https://example.com/project-image.jpg",
    commitMessage: "Add new feature: User authentication",
    date: "2025-07-13T17:09:26Z",
    branch: "main",
  },
])
</script>
<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <div class="flex gap-2">
        <DInput class="flex-1" placeholder="Search Projects..." />
        <DButton>Add Project</DButton>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <DProjectCard v-for="project in projects" :key="project.name" :project="project" />
      </div>
    </div>
  </DPageWrapper>
</template>
