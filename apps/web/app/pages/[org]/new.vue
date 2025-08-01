<script setup lang="ts">
const route = useRoute()
const router = useRouter()
const orgId = computed<string>(() => route.params.org as string)

// Form state
const formData = reactive({
  name: "",
  githubOwner: "",
  githubRepo: "",
  port: 3000,
  desiredRevisionSHA: "",
  env: [] as Array<{ name: string; value: string }>,
  basePath: "",
})

const error = ref<string | null>(null)
const isSubmitting = ref(false)
const showEnvPasteArea = ref(false)
const envPasteContent = ref("")

// Environment variable management
function addEnvVariable() {
  formData.env.push({ name: "", value: "" })
}

function removeEnvVariable(index: number) {
  formData.env.splice(index, 1)
}

// Parse environment variables from pasted content
function parseEnvVariables(content: string) {
  const lines = content.split("\n")
  const envVars: Array<{ name: string; value: string }> = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i]
    if (line === undefined) {
      i++
      continue
    }

    // Skip empty lines
    if (!line.trim()) {
      i++
      continue
    }

    // Skip comment lines (starting with #)
    if (line.trim().startsWith("#")) {
      i++
      continue
    }

    // Match KEY=VALUE pattern
    const match = line.match(/^([^=]+)=(.*)$/)
    if (match && match[1]) {
      let name = match[1].trim()
      let value = match[2] || ""

      // Check if value starts with a quote (single or double)
      const firstChar = value.trim()[0]
      if (firstChar === '"' || firstChar === "'") {
        // Multi-line value handling
        const quote = firstChar
        value = value.trim().substring(1) // Remove opening quote

        // Check if the value ends with the same quote on the same line
        if (value.endsWith(quote)) {
          // Single line quoted value
          value = value.slice(0, -1)
        } else {
          // Multi-line value - continue reading until we find the closing quote
          const valueLines = [value]
          i++

          while (i < lines.length) {
            const nextLine = lines[i]
            if (nextLine === undefined) break

            valueLines.push(nextLine)

            // Check if this line ends with the closing quote (not escaped)
            if (nextLine.endsWith(quote) && !nextLine.endsWith("\\" + quote)) {
              // Remove the closing quote from the last line
              valueLines[valueLines.length - 1] = nextLine.slice(0, -1)
              break
            }
            i++
          }

          // Join the lines with newlines preserved
          value = valueLines.join("\n")
        }

        // Handle escaped quotes within the value
        const escapedQuote = "\\" + quote
        value = value.replace(new RegExp(escapedQuote, "g"), quote)
      } else {
        // Non-quoted value - handle inline comments
        value = value.trim()
        if (value) {
          // Remove inline comments (anything after # that's not inside quotes)
          const hashIndex = value.indexOf("#")
          if (hashIndex !== -1) {
            value = value.substring(0, hashIndex).trim()
          }
        }
      }

      if (name) {
        envVars.push({ name, value })
      }
    }

    i++
  }

  return envVars
}

// Handle paste in environment variables section
function handleEnvPaste() {
  const parsed = parseEnvVariables(envPasteContent.value)
  if (parsed.length > 0) {
    // Add parsed variables to existing ones
    formData.env.push(...parsed)
    // Clear paste area and hide it
    envPasteContent.value = ""
    showEnvPasteArea.value = false
  }
}

// Handle paste event on individual env inputs
function handleEnvInputPaste(event: ClipboardEvent, index: number) {
  const pastedText = event.clipboardData?.getData("text")
  if (!pastedText || !pastedText.includes("\n")) return // Only handle multi-line paste

  event.preventDefault()
  const parsed = parseEnvVariables(pastedText)
  if (parsed.length > 0) {
    // Replace current empty entry with first parsed var
    const currentEnv = formData.env[index]
    if (currentEnv && !currentEnv.name && !currentEnv.value && parsed[0]) {
      formData.env[index] = parsed[0]
      // Add remaining vars
      if (parsed.length > 1) {
        formData.env.splice(index + 1, 0, ...parsed.slice(1))
      }
    } else {
      // Just add all parsed vars after current index
      formData.env.splice(index + 1, 0, ...parsed)
    }
  }
}

// Handle form submission
async function handleSubmit(event: Event) {
  event.preventDefault()
  error.value = null
  isSubmitting.value = true

  try {
    // Debug environment variables before sending
    const envVarsToSend = formData.env.filter((e) => e.name && e.value)
    if (envVarsToSend.length > 0) {
      console.log(`[Frontend] Sending ${envVarsToSend.length} environment variables`)
      envVarsToSend.forEach((env, index) => {
        const hasNewlines = env.value.includes("\n")
        console.log(`[${index}] ${env.name}: length=${env.value.length}, multiline=${hasNewlines}`)
        if (hasNewlines) {
          console.log(`  Line count: ${env.value.split("\n").length}`)
          console.log(`  First 100 chars: "${env.value.substring(0, 100).replace(/\n/g, "\\n")}..."`)
        }
      })
    }

    const response = await $fetch(`/api/organisations/${orgId.value}/projects`, {
      method: "POST",
      body: {
        name: formData.name,
        githubOwner: formData.githubOwner,
        githubRepo: formData.githubRepo,
        port: formData.port,
        desiredRevisionSHA: formData.desiredRevisionSHA || undefined,
        env: envVarsToSend,
        basePath: formData.basePath || undefined,
      },
    })

    // Redirect to project page on success
    await router.push(`/${orgId.value}/${response.id}`)
  } catch (err: any) {
    error.value = err.data?.message || "Failed to create project"
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="p-8">
    <div class="flex flex-col gap-4">
      <h1 class="text-title-md">Create New Project</h1>

      <form @submit="handleSubmit" class="flex flex-col gap-4 rounded-lg">
        <div class="flex flex-col gap-1.5">
          <label for="name">Project Name</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="name"
            v-model="formData.name"
            type="text"
            required
            placeholder="My Awesome Project"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="githubOwner">GitHub Owner</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="githubOwner"
            v-model="formData.githubOwner"
            type="text"
            required
            placeholder="username or organization"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="githubRepo">GitHub Repository</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="githubRepo"
            v-model="formData.githubRepo"
            type="text"
            required
            placeholder="repository-name"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="port">Port</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-2 text-sm"
            id="port"
            v-model.number="formData.port"
            type="number"
            required
            min="1"
            max="65535"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="desiredRevisionSHA">Initial Revision SHA (optional)</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-1.5 text-sm"
            id="desiredRevisionSHA"
            v-model="formData.desiredRevisionSHA"
            type="text"
            placeholder="Leave empty for latest"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="basePath">Base Path (optional)</label>
          <input
            class="border-neutral text-neutral rounded-md border px-2.5 py-1.5 text-sm"
            id="basePath"
            v-model="formData.basePath"
            type="text"
            placeholder="e.g., /api or /app (leave empty for root)"
          />
          <p class="text-xs text-neutral-600 dark:text-neutral-400">
            The base path in the repository where your Dockerfile is located.
          </p>
        </div>

        <div class="flex flex-col gap-1.5">
          <label>Environment Variables (optional)</label>
          <div class="flex flex-col gap-2">
            <div v-for="(envVar, index) in formData.env" :key="index" class="flex gap-2">
              <input
                class="border-neutral text-neutral flex-1 rounded-md border px-2.5 py-1.5 text-sm"
                v-model="envVar.name"
                type="text"
                placeholder="Variable name (e.g., NODE_ENV)"
                @paste="handleEnvInputPaste($event, index)"
              />
              <textarea
                class="border-neutral text-neutral flex-1 rounded-md border px-2.5 py-1.5 text-sm"
                v-model="envVar.value"
                type="text"
                placeholder="Value"
                cols="30"
              />
              <button
                type="button"
                @click="removeEnvVariable(index)"
                class="border-neutral rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                Remove
              </button>
            </div>

            <div class="flex gap-2">
              <button
                type="button"
                @click="addEnvVariable"
                class="border-neutral self-start rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                + Add Environment Variable
              </button>
              <button
                type="button"
                @click="showEnvPasteArea = !showEnvPasteArea"
                class="border-neutral self-start rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                📋 Paste from .env
              </button>
            </div>

            <div v-if="showEnvPasteArea" class="mt-2 flex flex-col gap-2">
              <p class="text-sm text-neutral-600 dark:text-neutral-400">
                Paste your .env content below. Comments (lines starting with # or inline after #) will be automatically
                removed.
              </p>
              <textarea
                v-model="envPasteContent"
                class="border-neutral text-neutral rounded-md border px-2.5 py-2 font-mono text-sm"
                rows="8"
                placeholder="NODE_ENV=production&#10;API_KEY=your-api-key&#10;# This is a comment&#10;DATABASE_URL=postgresql://... # inline comment&#10;MULTI_LINE_VALUE=&quot;This is a&#10;multi-line value&#10;that spans multiple lines&quot;&#10;PRIVATE_KEY='-----BEGIN RSA PRIVATE KEY-----&#10;MIIEpAIBAAKCAQ...&#10;-----END RSA PRIVATE KEY-----'"
              ></textarea>
              <div class="flex gap-2">
                <button
                  type="button"
                  @click="handleEnvPaste"
                  class="border-neutral rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-800"
                >
                  Parse & Add Variables
                </button>
                <button
                  type="button"
                  @click="
                    () => {
                      showEnvPasteArea = false
                      envPasteContent = ''
                    }
                  "
                  class="border-neutral rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-800"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>

        <div v-if="error">
          <p style="color: red">{{ error }}</p>
        </div>

        <div>
          <DButton type="submit" :disabled="isSubmitting" variant="primary" size="lg">
            {{ isSubmitting ? "Creating..." : "Create Project" }}
          </DButton>
        </div>
      </form>
    </div>
  </div>
</template>
