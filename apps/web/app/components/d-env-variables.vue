<script setup lang="ts">
export interface EnvVariable {
  name: string
  value: string
}

const props = defineProps<{
  modelValue: EnvVariable[]
}>()

const emit = defineEmits<{
  "update:modelValue": [value: EnvVariable[]]
}>()

const showEnvPasteArea = ref(false)
const envPasteContent = ref("")

// Environment variable management
function addEnvVariable() {
  const newEnv = [...props.modelValue, { name: "", value: "" }]
  emit("update:modelValue", newEnv)
}

function removeEnvVariable(index: number) {
  const newEnv = [...props.modelValue]
  newEnv.splice(index, 1)
  emit("update:modelValue", newEnv)
}

function updateEnvVariable(index: number, field: "name" | "value", value: string) {
  const newEnv = [...props.modelValue]
  const currentItem = newEnv[index]
  if (!currentItem) return

  if (field === "name") {
    newEnv[index] = { name: value, value: currentItem.value }
  } else {
    newEnv[index] = { name: currentItem.name, value: value }
  }
  emit("update:modelValue", newEnv)
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
    const newEnv = [...props.modelValue, ...parsed]
    emit("update:modelValue", newEnv)
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
    const newEnv = [...props.modelValue]
    // Replace current empty entry with first parsed var
    const currentEnv = newEnv[index]
    if (currentEnv && !currentEnv.name && !currentEnv.value && parsed[0]) {
      newEnv[index] = parsed[0]
      // Add remaining vars
      if (parsed.length > 1) {
        newEnv.splice(index + 1, 0, ...parsed.slice(1))
      }
    } else {
      // Just add all parsed vars after current index
      newEnv.splice(index + 1, 0, ...parsed)
    }
    emit("update:modelValue", newEnv)
  }
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div v-for="(envVar, index) in modelValue" :key="index" class="flex gap-2">
      <input
        class="border-neutral text-neutral flex-1 rounded-md border px-2.5 py-1.5 text-sm"
        :value="envVar.name"
        @input="updateEnvVariable(index, 'name', ($event.target as HTMLInputElement).value)"
        type="text"
        placeholder="Variable name (e.g., NODE_ENV)"
        @paste="handleEnvInputPaste($event, index)"
      />
      <textarea
        class="border-neutral text-neutral flex-1 rounded-md border px-2.5 py-1.5 text-sm"
        :value="envVar.value"
        @input="updateEnvVariable(index, 'value', ($event.target as HTMLTextAreaElement).value)"
        placeholder="Value"
        rows="1"
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
        Paste your .env content below. Comments (lines starting with # or inline after #) will be automatically removed.
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
</template>
