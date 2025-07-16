<script setup lang="ts">
import { useQuery } from "@urql/vue"
import { graphql } from "~/gql"

const route = useRoute()
const orgId = route.params.org

// const { data, fetching, error } = useQuery({
//   query: graphql(/* GraphQL */ `
//     query Projects($orgId: ID!) {
//       projects(input: { organisationId: $orgId }) {
//         nodes {
//           ...Project_ProjectFragment
//         }
//       }
//     }
//   `),
//   variables: {
//     orgId,
//   },
// })

const { data: me } = useQuery({
  query: graphql(/* GraphQL */ `
    query Me {
      me {
        user {
          id
          username
          githubId
          organisations {
            nodes {
              id
              name
              slug
            }
          }
        }
      }
    }
  `),
})

// const projects = computed(() => data.value?.projects?.nodes)
const projects = computed(() => [])
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <div class="flex gap-2">
        <DInput class="flex-1" placeholder="Search Projects..." />
        <DButton>Add Project</DButton>
      </div>
      <pre>{{ me }}</pre>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <DProjectCard v-for="project in projects" :key="project.id" :project="project" />
      </div>
    </div>
  </DPageWrapper>
</template>
