<script setup lang="ts">
import { useMutation, useQuery } from "@urql/vue"
import { graphql } from "~/gql"

const route = useRoute()
const router = useRouter()
const orgId = computed<string>(() => route.params.org as string)

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
  variables: {},
})

const organisations = computed(() => me.value?.me?.user?.organisations?.nodes || [])
const selectedOrganisation = ref<string>("")

// Initialize selected organisation when data loads
watch(organisations, (newOrgs) => {
  if (newOrgs.length > 0 && !selectedOrganisation.value) {
    // Find the current org from route or use first one
    const currentOrg = newOrgs.find((org) => org?.slug === orgId.value)
    selectedOrganisation.value = currentOrg?.id || newOrgs[0]?.id || ""
  }
})

const { executeMutation: createProject, fetching } = useMutation(
  graphql(/* GraphQL */ `
    mutation CreateProject($input: CreateProjectInput!) {
      createProject(input: $input) {
        project {
          id
          name
          slug
          organisation {
            id
            name
            slug
          }
        }
      }
    }
  `),
)

// Form data
const project = ref({
  name: "",
  githubOwner: "",
  githubRepo: "",
  rootDirectory: "./",
  dockerfile: "./Dockerfile",
})

// Environment variables
interface EnvVariable {
  key: string
  value: string
}

const envVariables = ref<EnvVariable[]>([{ key: "", value: "" }])

function addEnvVariable() {
  envVariables.value.push({ key: "", value: "" })
}

function removeEnvVariable(index: number) {
  envVariables.value.splice(index, 1)
}

async function handleFormSubmit() {
  // Validate required fields
  if (!project.value.name || !project.value.githubOwner || !project.value.githubRepo) {
    alert("Please fill in all required fields")
    return
  }

  const result = await createProject({
    input: {
      name: project.value.name,
      githubOwner: project.value.githubOwner,
      githubRepo: project.value.githubRepo,
    },
  })

  if (result.data?.createProject?.project) {
    const newProject = result.data.createProject.project
    // Navigate to the new project page
    await router.push(`/${newProject.organisation.slug}/${newProject.slug}`)
  } else if (result.error) {
    alert(`Error creating project: ${result.error.message}`)
  }
}

// Extract GitHub info from URL or provide input fields
const githubInfo = ref({
  owner: "dokedu",
  repo: "dokedu",
  branch: "main",
})

// Watch for changes in GitHub fields to update the display
watchEffect(() => {
  if (project.value.githubOwner && project.value.githubRepo) {
    githubInfo.value.owner = project.value.githubOwner
    githubInfo.value.repo = project.value.githubRepo
  }
})
</script>

<template>
  <DPageWrapper>
    <div class="flex flex-col gap-4 py-12">
      <div class="mx-auto w-full max-w-2xl">
        <h1 class="text-title mb-8 font-semibold">New Project</h1>

        <div class="border-neutral bg-neutral rounded-lg border p-6 shadow-sm">
          <div class="mb-6">
            <p class="text-neutral mb-2">Importing from GitHub</p>
            <div class="text-copy-sm flex items-center gap-2">
              <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
                <path
                  d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"
                />
              </svg>
              <span class="font-medium">{{ githubInfo.owner }}/{{ githubInfo.repo }}</span>
              <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M13 7l5 5m0 0l-5 5m5-5H6"
                ></path>
              </svg>
              <span class="text-neutral-subtle">{{ githubInfo.branch }}</span>
            </div>
          </div>

          <form @submit.prevent="handleFormSubmit" class="space-y-6">
            <p class="text-neutral">Choose where you want to create the project and give it a name.</p>

            <!-- GitHub Repository Info -->
            <div class="grid grid-cols-2 gap-6">
              <div>
                <label class="text-label text-neutral mb-2 block font-medium">GitHub Owner</label>
                <input
                  v-model="project.githubOwner"
                  type="text"
                  required
                  placeholder="owner"
                  class="border-neutral bg-neutral text-copy text-neutral hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                />
              </div>

              <div>
                <label class="text-label text-neutral mb-2 block font-medium">GitHub Repository</label>
                <input
                  v-model="project.githubRepo"
                  type="text"
                  required
                  placeholder="repository"
                  class="border-neutral bg-neutral text-copy text-neutral hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                />
              </div>
            </div>

            <div class="grid grid-cols-2 gap-6">
              <div>
                <label class="text-label text-neutral mb-2 block font-medium">Zeitwork Team</label>
                <div class="relative">
                  <select
                    v-model="selectedOrganisation"
                    class="bg-neutral border-neutral text-copy text-neutral hover:border-neutral-strong/30 w-full appearance-none rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                  >
                    <option v-for="org in organisations" :key="org.id" :value="org.id">
                      {{ org.name }}
                    </option>
                  </select>
                  <svg
                    class="text-neutral-subtle pointer-events-none absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2 transform"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
                  </svg>
                </div>
              </div>

              <div>
                <label class="text-label text-neutral mb-2 block font-medium">Project Name</label>
                <div class="flex items-center">
                  <span class="text-neutral-subtle mr-2">/</span>
                  <input
                    v-model="project.name"
                    type="text"
                    required
                    placeholder="my-project"
                    class="border-neutral bg-neutral text-copy text-neutral hover:border-neutral-strong/30 flex-1 rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                  />
                </div>
              </div>
            </div>

            <div>
              <label class="text-label text-neutral mb-2 block font-medium">Root Directory</label>
              <input
                v-model="project.rootDirectory"
                type="text"
                class="border-neutral bg-neutral text-copy text-neutral hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
              />
            </div>

            <div>
              <label class="text-label text-neutral mb-2 block font-medium">Dockerfile</label>
              <div class="relative">
                <input
                  v-model="project.dockerfile"
                  type="text"
                  class="border-neutral bg-neutral text-copy text-neutral hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 pr-10 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                />
                <div class="absolute top-1/2 right-3 -translate-y-1/2 transform">
                  <div class="bg-accent flex h-6 w-6 items-center justify-center rounded-full">
                    <svg class="text-accent-onsurface h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path>
                    </svg>
                  </div>
                </div>
              </div>
            </div>

            <div>
              <details>
                <summary class="text-neutral flex cursor-pointer items-center gap-2 font-medium">
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
                  </svg>
                  Environment Variables
                </summary>
                <div class="mt-4 space-y-4">
                  <div v-for="(env, index) in envVariables" :key="index" class="grid grid-cols-2 gap-4">
                    <div>
                      <label class="text-label text-neutral mb-2 block font-medium">Key</label>
                      <input
                        v-model="env.key"
                        type="text"
                        placeholder="EXAMPLE_NAME"
                        class="border-neutral bg-neutral text-copy text-neutral placeholder-neutral-weak hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                      />
                    </div>
                    <div class="flex gap-2">
                      <div class="flex-1">
                        <label class="text-label text-neutral mb-2 block font-medium">Value</label>
                        <input
                          v-model="env.value"
                          type="text"
                          placeholder="123456789"
                          class="border-neutral bg-neutral text-copy text-neutral placeholder-neutral-weak hover:border-neutral-strong/30 w-full rounded-md border px-4 py-2 focus:border-blue-600 focus:ring-2 focus:ring-blue-300 focus:outline-none"
                        />
                      </div>
                      <button
                        v-if="envVariables.length > 1"
                        @click="removeEnvVariable(index)"
                        type="button"
                        class="text-neutral-subtle hover:text-neutral self-end p-2"
                      >
                        <svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4"></path>
                        </svg>
                      </button>
                    </div>
                  </div>

                  <button
                    @click="addEnvVariable"
                    type="button"
                    class="text-neutral hover:text-neutral-strong flex items-center gap-2 font-medium"
                  >
                    <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M12 6v6m0 0v6m0-6h6m-6 0H6"
                      ></path>
                    </svg>
                    Add More
                  </button>

                  <p class="text-copy-sm text-neutral-subtle">Tip: Paste an .env above to populate the form.</p>
                </div>
              </details>
            </div>

            <button
              type="submit"
              :disabled="fetching"
              class="bg-neutral-inverse text-neutral-inverse hover:bg-neutral-inverse-hover w-full rounded-md py-3 font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            >
              {{ fetching ? "Deploying..." : "Deploy" }}
            </button>
          </form>
        </div>
      </div>
    </div>
  </DPageWrapper>
</template>
