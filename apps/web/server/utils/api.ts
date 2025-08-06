import * as k8s from "@kubernetes/client-node"
import { useDrizzle, eq, and } from "./drizzle"
import * as schema from "~~/packages/database/schema"
import { randomBytes } from "crypto"
import { addDays } from "date-fns"
import { getRepository, getLatestCommitSHA } from "./github"
import * as https from "https"

type ZeitworkResponse<T> =
  | {
      data: null
      error: Error
    }
  | {
      data: T
      error: null
    }

// Kubernetes types for custom resources
interface EnvVar {
  name: string
  value: string
}

interface AppSpec {
  description: string
  desiredRevision?: string
  fqdn?: string
  githubInstallation: number
  githubOwner: string
  githubRepo: string
  port: number
  env?: EnvVar[]
  basePath?: string
}

interface AppStatus {
  currentRevision?: string
}

interface App extends k8s.KubernetesObject {
  spec: AppSpec
  status?: AppStatus
}

interface AppRevisionSpec {
  commitSHA: string
}

interface AppRevision extends k8s.KubernetesObject {
  spec: AppRevisionSpec
  status?: any
}

// Initialize Kubernetes client
function getK8sClient() {
  const config = useRuntimeConfig()

  const kc = new k8s.KubeConfig()

  try {
    // Parse and potentially fix the kubeconfig
    let kubeconfigString = config.kubeConfig

    kc.loadFromString(kubeconfigString)
  } catch (error) {
    console.error("Failed to load kubeconfig:", error)
    throw new Error("Failed to load kubeconfig")
  }

  return {
    coreV1Api: kc.makeApiClient(k8s.CoreV1Api),
    customObjectsApi: kc.makeApiClient(k8s.CustomObjectsApi),
    config: kc,
  }
}

// Helper function to properly escape environment variable values
function escapeEnvVarValue(value: string): string {
  // Kubernetes expects newlines to be preserved in the JSON payload
  // The value should already have actual newline characters from the frontend
  // We need to ensure they're preserved when sent to Kubernetes

  // Log if we have multi-line values for debugging
  if (value.includes("\n")) {
    console.log(`[escapeEnvVarValue] Processing multi-line value with ${value.split("\n").length} lines`)
    console.log(`[escapeEnvVarValue] First 100 chars: "${value.substring(0, 100)}..."`)
  }

  // The value should be sent as-is to Kubernetes
  // JSON.stringify will handle escaping when the request is made
  return value
}

// Helper function to log environment variables for debugging
function logEnvVars(envVars: EnvVar[] | undefined, context: string) {
  if (!envVars || envVars.length === 0) return

  console.log(`[${context}] Environment variables:`)
  envVars.forEach((env, index) => {
    const valuePreview = env.value.length > 50 ? env.value.substring(0, 50) + "..." : env.value
    const hasNewlines = env.value.includes("\n")
    console.log(`  [${index}] ${env.name}: "${valuePreview}" (length: ${env.value.length}, multiline: ${hasNewlines})`)
    if (hasNewlines) {
      console.log(`    Line count: ${env.value.split("\n").length}`)
    }
  })
}

// Helper function to create or update namespace
async function ensureNamespace(organisationNo: number): Promise<void> {
  const { coreV1Api } = getK8sClient()
  const namespaceName = `tenant-${organisationNo}`

  try {
    await coreV1Api.readNamespace({ name: namespaceName })
    console.log(`Namespace ${namespaceName} already exists`)
  } catch (error: any) {
    // Check for 404 in multiple possible locations in the error object
    const is404 =
      error.response?.statusCode === 404 ||
      error.statusCode === 404 ||
      error.body?.code === 404 ||
      error.code === 404 ||
      (error.response?.body && typeof error.response.body === "object" && error.response.body.code === 404) ||
      (error.response?.body && typeof error.response.body === "string" && error.response.body.includes('"code":404')) ||
      (error.message && error.message.includes("404")) ||
      (error.message && error.message.includes("not found")) ||
      error.response?.body?.reason === "NotFound"

    if (is404) {
      // Namespace doesn't exist, create it
      console.log(`Creating namespace ${namespaceName}`)
      const namespace: k8s.V1Namespace = {
        metadata: {
          name: namespaceName,
          labels: {
            "zeitwork.com/organisation-id": organisationNo.toString(),
          },
        },
      }
      try {
        const result = await coreV1Api.createNamespace({ body: namespace })
        console.log(`Namespace ${namespaceName} created successfully`)
      } catch (createError: any) {
        console.error(`Failed to create namespace ${namespaceName}:`, createError.response?.body || createError)
        console.error(`Create error details:`, {
          statusCode: createError.response?.statusCode || createError.statusCode,
          body: createError.response?.body || createError.body,
          message: createError.message,
        })
        throw new Error(`Failed to create namespace: ${createError.response?.body?.message || createError.message}`)
      }
    } else {
      console.error(`Failed to check namespace ${namespaceName}:`, error.response?.body || error)
      console.error(`Full error object:`, JSON.stringify(error, null, 2))
      throw new Error(`Failed to check namespace: ${error.response?.body?.message || error.message}`)
    }
  }
}

// Helper to create or update App custom resource
async function createOrUpdateApp(
  namespace: string,
  appName: string,
  organisationNo: number,
  spec: AppSpec,
): Promise<void> {
  const { customObjectsApi } = getK8sClient()
  const group = "zeitwork.com"
  const version = "v1alpha1"
  const plural = "apps"

  // Process environment variables to ensure proper escaping
  if (spec.env) {
    logEnvVars(spec.env, "Before processing")

    // Ensure values are properly formatted
    spec.env = spec.env.map((env) => ({
      name: env.name,
      value: escapeEnvVarValue(env.value),
    }))

    logEnvVars(spec.env, "After processing")
  }

  const app = {
    apiVersion: `${group}/${version}`,
    kind: "App",
    metadata: {
      name: appName,
      namespace: namespace,
      labels: {
        "zeitwork.com/organisationId": organisationNo.toString(),
      },
    },
    spec: spec,
  }

  console.log(`Creating/updating app ${appName} in namespace ${namespace}`)
  if (spec.env && spec.env.length > 0) {
    console.log(`App has ${spec.env.length} environment variables`)
  }

  try {
    // Try to get existing app
    await customObjectsApi.getNamespacedCustomObject({
      group,
      version,
      namespace,
      plural,
      name: appName,
    })
    // If it exists, patch it
    // Use JSON merge patch to ensure proper handling of multi-line values
    const patchOptions = {
      group,
      version,
      namespace,
      plural,
      name: appName,
      body: app,
      headers: {
        "Content-Type": "application/merge-patch+json",
      },
    } as any

    console.log(`Patching app ${appName} with JSON merge patch`)
    await customObjectsApi.patchNamespacedCustomObject(patchOptions)
  } catch (error: any) {
    // Check for 404 in multiple possible locations in the error object
    const is404 =
      error.response?.statusCode === 404 ||
      error.statusCode === 404 ||
      error.body?.code === 404 ||
      error.code === 404 ||
      (error.response?.body && typeof error.response.body === "string" && error.response.body.includes('"code":404')) ||
      (error.message && error.message.includes("404"))

    if (is404) {
      // App doesn't exist, create it
      console.log(`Creating new app ${appName}`)
      await customObjectsApi.createNamespacedCustomObject({
        group,
        version,
        namespace,
        plural,
        body: app,
      })
      console.log(`Successfully created app ${appName}`)
    } else {
      console.error(`Failed to check/update app ${appName}:`, error.response?.body || error)
      throw error
    }
  }

  // Verify the app was created/updated with correct env vars
  if (spec.env && spec.env.length > 0) {
    try {
      const verifyResponse = await customObjectsApi.getNamespacedCustomObject({
        group,
        version,
        namespace,
        plural,
        name: appName,
      })

      const verifyData = verifyResponse.body || verifyResponse
      const storedApp = verifyData as App

      if (storedApp.spec.env) {
        console.log(`[Verification] App ${appName} has ${storedApp.spec.env.length} env vars stored`)
        logEnvVars(storedApp.spec.env, "Stored in Kubernetes")

        // Check if any multi-line values were truncated
        storedApp.spec.env.forEach((env, index) => {
          const originalEnv = spec.env?.find((e) => e.name === env.name)
          if (originalEnv && originalEnv.value !== env.value) {
            console.error(`[ERROR] Env var ${env.name} was modified during storage!`)
            console.error(`  Original length: ${originalEnv.value.length}, Stored length: ${env.value.length}`)
            if (originalEnv.value.includes("\n")) {
              console.error(
                `  Original had ${originalEnv.value.split("\n").length} lines, stored has ${env.value.split("\n").length} lines`,
              )
            }
          }
        })
      }
    } catch (verifyError) {
      console.error("Failed to verify app creation:", verifyError)
    }
  }
}

export function useZeitworkClient() {
  // Get the user ID from the current session
  async function getCurrentUserId(): Promise<string | null> {
    // This is called from API routes where we already have the user from requireUserSession
    // The user ID will need to be passed in from the API route
    return null
  }

  interface Organisation {
    id: string
    no: number
    name: string
    slug: string
    installationId?: number
  }

  interface CreateOrganisationInput {
    name: string
    slug: string
  }

  async function createOrganisation(options: CreateOrganisationInput): Promise<ZeitworkResponse<Organisation>> {
    try {
      const db = useDrizzle()
      const [organisation] = await db
        .insert(schema.organisations)
        .values({
          name: options.name,
          slug: options.slug,
        })
        .returning()

      if (!organisation) {
        return { data: null, error: new Error("Failed to create organisation") }
      }

      return {
        data: {
          id: organisation.id,
          no: organisation.no,
          name: organisation.name,
          slug: organisation.slug,
          installationId: organisation.installationId || undefined,
        },
        error: null,
      }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface GetOrganisationOptions {
    organisationIdOrSlug: string
    userId: string
  }

  async function getOrganisation(options: GetOrganisationOptions): Promise<ZeitworkResponse<Organisation>> {
    try {
      const db = useDrizzle()

      // First, try to find the organisation by slug or ID
      let organisation = null

      // Check if the parameter looks like a UUID (simple check for UUID v4/v7 format)
      const isUuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
        options.organisationIdOrSlug,
      )

      if (isUuid) {
        // Try to find by ID first if it looks like a UUID
        const [orgById] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.id, options.organisationIdOrSlug))
          .limit(1)
        organisation = orgById
      } else {
        // Try to find by slug if it doesn't look like a UUID
        const [orgBySlug] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.slug, options.organisationIdOrSlug))
          .limit(1)
        organisation = orgBySlug
      }

      if (!organisation) {
        return { data: null, error: new Error("Organisation not found") }
      }

      // Check if user has access to this organisation
      const [memberRecord] = await db
        .select()
        .from(schema.organisationMembers)
        .where(
          and(
            eq(schema.organisationMembers.userId, options.userId),
            eq(schema.organisationMembers.organisationId, organisation.id),
          ),
        )
        .limit(1)

      if (!memberRecord) {
        return { data: null, error: new Error("Organisation not found or access denied") }
      }

      return {
        data: {
          id: organisation.id,
          no: organisation.no,
          name: organisation.name,
          slug: organisation.slug,
          installationId: organisation.installationId || undefined,
        },
        error: null,
      }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface ListOrganisationsOptions {
    userId: string
  }

  async function listOrganisations(options: ListOrganisationsOptions): Promise<ZeitworkResponse<Organisation[]>> {
    try {
      const db = useDrizzle()

      // Get all organisations the user is a member of
      const organisations = await db
        .select({
          id: schema.organisations.id,
          no: schema.organisations.no,
          name: schema.organisations.name,
          slug: schema.organisations.slug,
          installationId: schema.organisations.installationId,
        })
        .from(schema.organisations)
        .innerJoin(schema.organisationMembers, eq(schema.organisations.id, schema.organisationMembers.organisationId))
        .where(eq(schema.organisationMembers.userId, options.userId))

      return {
        data: organisations.map((org) => ({
          ...org,
          installationId: org.installationId || undefined,
        })),
        error: null,
      }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface CreateProjectInput {
    organisationId: string
    name: string
    githubOwner: string
    githubRepo: string
    port: number
    desiredRevisionSHA?: string
    domain?: string
    env?: EnvVar[]
    basePath?: string
  }

  interface Project {
    id: string
    k8sName: string
    name: string
    organisationId: string
    githubOwner: string
    githubRepo: string
    port: number
    domain?: string
    basePath?: string
  }

  async function createProject(options: CreateProjectInput): Promise<ZeitworkResponse<Project>> {
    try {
      const db = useDrizzle()

      // Log incoming environment variables
      if (options.env && options.env.length > 0) {
        console.log(`[createProject] Received ${options.env.length} environment variables`)
        logEnvVars(options.env, "createProject input")
      }

      // Get the organisation to check installation ID
      const [organisation] = await db
        .select()
        .from(schema.organisations)
        .where(eq(schema.organisations.id, options.organisationId))
        .limit(1)

      if (!organisation) {
        return { data: null, error: new Error("Organisation not found") }
      }

      if (!organisation.installationId) {
        return { data: null, error: new Error("GitHub installation not configured for this organisation") }
      }

      // Fetch GitHub repository information
      let repoInfo: { id: number; defaultBranch: string }
      try {
        repoInfo = await getRepository(organisation.installationId, options.githubOwner, options.githubRepo)
      } catch (error: any) {
        console.error(`Failed to fetch GitHub repository:`, error)
        return { data: null, error: new Error(`Failed to fetch GitHub repository: ${error.message}`) }
      }

      // Get the latest commit SHA if not provided
      let desiredRevisionSHA = options.desiredRevisionSHA
      if (!desiredRevisionSHA) {
        try {
          desiredRevisionSHA = await getLatestCommitSHA(
            organisation.installationId,
            options.githubOwner,
            options.githubRepo,
            repoInfo.defaultBranch,
          )
        } catch (error: any) {
          console.error(`Failed to fetch latest commit SHA:`, error)
          return { data: null, error: new Error(`Failed to fetch latest commit SHA: ${error.message}`) }
        }
      }

      // Ensure namespace exists for the organisation
      try {
        await ensureNamespace(organisation.no)
      } catch (namespaceError: any) {
        console.error(`Failed to ensure namespace for organisation ${organisation.id}:`, namespaceError)
        return { data: null, error: new Error(`Failed to create namespace: ${namespaceError.message}`) }
      }

      const namespace = `tenant-${organisation.no}`
      // Use the numeric repo ID just like the Go implementation
      const appName = `repo-${repoInfo.id}`

      // Create or update the app
      await createOrUpdateApp(namespace, appName, organisation.no, {
        description: options.name,
        desiredRevision: desiredRevisionSHA,
        githubInstallation: organisation.installationId,
        githubOwner: options.githubOwner,
        githubRepo: options.githubRepo,
        port: options.port,
        fqdn: options.domain,
        env: options.env,
        basePath: options.basePath,
      })

      const project: Project = {
        id: `${namespace}/${appName}`, // Match Go format: namespace/name
        k8sName: appName,
        name: options.name,
        organisationId: options.organisationId,
        githubOwner: options.githubOwner,
        githubRepo: options.githubRepo,
        port: options.port,
        domain: options.domain,
        basePath: options.basePath,
      }

      return { data: project, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface ListProjectsOptions {
    organisationId: string
    organisationNo: number
  }

  async function listProjects(options: ListProjectsOptions): Promise<ZeitworkResponse<Project[]>> {
    try {
      const { customObjectsApi, coreV1Api } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      // Check if namespace exists
      try {
        await coreV1Api.readNamespace({ name: namespace })
      } catch (error: any) {
        // Log detailed error information for debugging
        console.error(`Error accessing namespace ${namespace}:`, {
          message: error.message,
          code: error.code,
          statusCode: error.response?.statusCode || error.statusCode,
          body: error.response?.body || error.body,
        })

        // Check for 404 in multiple possible locations in the error object
        if (
          error.response?.statusCode === 404 ||
          error.statusCode === 404 ||
          error.body?.code === 404 ||
          (error.message && error.message.includes("404"))
        ) {
          // Namespace doesn't exist, return empty list
          return { data: [], error: null }
        }

        // For TLS/certificate errors, provide more helpful error message
        if (error.message && error.message.includes("unable to verify the first certificate")) {
          return {
            data: null,
            error: new Error(
              `TLS certificate verification failed. The Kubernetes cluster's CA certificate may not be properly configured in the kubeconfig.`,
            ),
          }
        }

        return { data: null, error: new Error(`Failed to access organisation namespace: ${error.message}`) }
      }

      // List apps in the namespace
      try {
        const response = await customObjectsApi.listNamespacedCustomObject({
          group: "zeitwork.com",
          version: "v1alpha1",
          namespace,
          plural: "apps",
        })

        // The response structure might vary, so check for items in different locations
        const responseData = response.body || response
        const items = (responseData as any).items || []

        const apps = items as App[]
        const projects: Project[] = apps.map((app) => ({
          id: `${namespace}/${app.metadata?.name || ""}`, // Match Go format
          k8sName: app.metadata?.name || "",
          name: app.spec.description,
          organisationId: options.organisationId,
          githubOwner: app.spec.githubOwner,
          githubRepo: app.spec.githubRepo,
          port: app.spec.port,
          domain: app.spec.fqdn,
          basePath: app.spec.basePath,
        }))

        return { data: projects, error: null }
      } catch (error: any) {
        return { data: null, error: new Error(`Failed to list projects: ${error.message}`) }
      }
    } catch (error: any) {
      return { data: null, error: new Error(`Kubernetes client error: ${error.message}`) }
    }
  }

  interface GetProjectOptions {
    organisationId: string
    organisationNo: number
    projectId: string
  }

  async function getProject(options: GetProjectOptions): Promise<ZeitworkResponse<Project>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      const response = await customObjectsApi.getNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
        name: options.projectId,
      })

      const responseData = response.body || response
      const app = responseData as App
      const project: Project = {
        id: `${namespace}/${app.metadata?.name || ""}`, // Match Go format
        k8sName: app.metadata?.name || "",
        name: app.spec.description,
        organisationId: options.organisationId,
        githubOwner: app.spec.githubOwner,
        githubRepo: app.spec.githubRepo,
        port: app.spec.port,
        domain: app.spec.fqdn,
        basePath: app.spec.basePath,
      }

      return { data: project, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface UpdateProjectInput {
    organisationId: string
    organisationNo: number
    projectId: string
    domain?: string
    env?: EnvVar[]
    basePath?: string
  }

  async function updateProject(options: UpdateProjectInput): Promise<ZeitworkResponse<Project>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      // Prepare JSON Patch operations
      const patchOps: any[] = []

      // Handle domain updates
      if (options.domain !== undefined) {
        if (options.domain === "" || options.domain === null) {
          patchOps.push({ op: "remove", path: "/spec/fqdn" })
        } else {
          patchOps.push({ op: "replace", path: "/spec/fqdn", value: options.domain })
        }
      }

      // Handle basePath updates
      if (options.basePath !== undefined) {
        if (options.basePath === "" || options.basePath === null) {
          patchOps.push({ op: "remove", path: "/spec/basePath" })
        } else {
          patchOps.push({ op: "replace", path: "/spec/basePath", value: options.basePath })
        }
      }

      // Handle environment variable updates
      if (options.env !== undefined) {
        // Process environment variables
        let processedEnv = options.env
        if (processedEnv && processedEnv.length > 0) {
          logEnvVars(processedEnv, "updateProject input")
          processedEnv = processedEnv.map((env) => ({
            name: env.name,
            value: escapeEnvVarValue(env.value),
          }))
          logEnvVars(processedEnv, "updateProject after processing")
        }

        if (processedEnv.length === 0) {
          // Remove env array if empty
          patchOps.push({ op: "remove", path: "/spec/env" })
        } else {
          // Replace entire env array
          patchOps.push({ op: "replace", path: "/spec/env", value: processedEnv })
        }
      }

      if (patchOps.length === 0) {
        // No updates to perform
        return getProject({
          organisationId: options.organisationId,
          organisationNo: options.organisationNo,
          projectId: options.projectId,
        })
      }

      // Apply the patch using JSON Patch format
      const patchOptions = {
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
        name: options.projectId,
        body: patchOps,
        headers: {
          "Content-Type": "application/json-patch+json",
        },
      } as any

      console.log(`Patching app ${options.projectId} with JSON patch operations:`, JSON.stringify(patchOps, null, 2))

      try {
        await customObjectsApi.patchNamespacedCustomObject(patchOptions)
      } catch (patchError: any) {
        // If replace fails because field doesn't exist, retry with add operations
        const needsRetry = patchError.response?.body?.message?.includes("does not exist")
        if (needsRetry) {
          console.log("Some replace operations failed, retrying with add operations where needed")

          // Get current app to check which fields exist
          const currentResponse = await customObjectsApi.getNamespacedCustomObject({
            group: "zeitwork.com",
            version: "v1alpha1",
            namespace,
            plural: "apps",
            name: options.projectId,
          })
          const currentApp = (currentResponse.body || currentResponse) as App

          // Rebuild patch operations based on what exists
          const retryOps = patchOps.map((op) => {
            if (op.op === "replace") {
              // Check if the field exists in current app
              if (op.path === "/spec/fqdn" && !currentApp.spec.fqdn) {
                return { ...op, op: "add" }
              }
              if (op.path === "/spec/basePath" && !currentApp.spec.basePath) {
                return { ...op, op: "add" }
              }
              if (op.path === "/spec/env" && !currentApp.spec.env) {
                return { ...op, op: "add" }
              }
            }
            return op
          })

          console.log(`Retrying with adjusted operations:`, JSON.stringify(retryOps, null, 2))
          await customObjectsApi.patchNamespacedCustomObject({
            ...patchOptions,
            body: retryOps,
          })
        } else {
          throw patchError
        }
      }

      // Verify the update if env vars were changed
      if (options.env !== undefined && options.env.length > 0) {
        const verifyResponse = await customObjectsApi.getNamespacedCustomObject({
          group: "zeitwork.com",
          version: "v1alpha1",
          namespace,
          plural: "apps",
          name: options.projectId,
        })

        const verifiedApp = (verifyResponse.body || verifyResponse) as App
        if (verifiedApp.spec.env) {
          console.log(`[Verification] App ${options.projectId} now has ${verifiedApp.spec.env.length} env vars`)
          logEnvVars(verifiedApp.spec.env, "Updated in Kubernetes")
        }
      }

      // Return the updated project
      const updatedResponse = await customObjectsApi.getNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
        name: options.projectId,
      })

      const updatedApp = (updatedResponse.body || updatedResponse) as App
      const project: Project = {
        id: `${namespace}/${updatedApp.metadata?.name || ""}`,
        k8sName: updatedApp.metadata?.name || "",
        name: updatedApp.spec.description,
        organisationId: options.organisationId,
        githubOwner: updatedApp.spec.githubOwner,
        githubRepo: updatedApp.spec.githubRepo,
        port: updatedApp.spec.port,
        domain: updatedApp.spec.fqdn,
        basePath: updatedApp.spec.basePath,
      }

      return { data: project, error: null }
    } catch (error) {
      console.error(`Failed to update project:`, error)
      return { data: null, error: error as Error }
    }
  }

  interface DeployProjectInput {
    organisationId: string
    organisationNo: number
    projectId: string
    commitSHA: string
  }

  async function deployProject(options: DeployProjectInput): Promise<ZeitworkResponse<{ deploymentId: string }>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      // Update the App's desiredRevision to trigger a deployment
      const patchOps = [{ op: "replace", path: "/spec/desiredRevision", value: options.commitSHA }]

      const patchOptions = {
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
        name: options.projectId,
        body: patchOps,
        headers: {
          "Content-Type": "application/json-patch+json",
        },
      } as any

      console.log(`Deploying project ${options.projectId} with commit SHA: ${options.commitSHA}`)

      try {
        await customObjectsApi.patchNamespacedCustomObject(patchOptions)
      } catch (patchError: any) {
        // If replace fails because field doesn't exist, try add operation
        if (patchError.response?.body?.message?.includes("does not exist")) {
          console.log("Replace failed, trying add operation")
          patchOps[0] = { op: "add", path: "/spec/desiredRevision", value: options.commitSHA }
          await customObjectsApi.patchNamespacedCustomObject({
            ...patchOptions,
            body: patchOps,
          })
        } else {
          throw patchError
        }
      }

      // Generate a deployment ID based on the commit SHA and timestamp
      const deploymentId = `${options.commitSHA.substring(0, 7)}-${Date.now()}`

      return { data: { deploymentId }, error: null }
    } catch (error) {
      console.error(`Failed to deploy project:`, error)
      return { data: null, error: error as Error }
    }
  }

  async function installGitHubForOrganisation(options: any): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface Deployment {
    id: string
    previewURL: string
    projectId: string
    organisationId: string
  }

  interface GetDeploymentOptions {
    projectId: string
    deploymentId: string
    organisationId: string
    organisationNo: number
  }

  async function getDeployment(options: GetDeploymentOptions): Promise<ZeitworkResponse<Deployment>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      const response = await customObjectsApi.getNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apprevisions",
        name: options.deploymentId,
      })

      const responseData = response.body || response
      const revision = responseData as AppRevision
      const deployment: Deployment = {
        id: revision.metadata?.name || "",
        previewURL: `${revision.spec.commitSHA.substring(0, 7)}-${options.projectId}.zeitwork.app`,
        projectId: options.projectId,
        organisationId: options.organisationId,
      }

      return { data: deployment, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface ListDeploymentsOptions {
    projectId: string
    organisationId: string
    organisationNo: number
  }

  async function listDeployments(options: ListDeploymentsOptions): Promise<ZeitworkResponse<Deployment[]>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `tenant-${options.organisationNo}`

      // List app revisions with label selector
      // Note: projectId should be the k8sName (e.g., "repo-123" or "repo-my-app")
      const response = await customObjectsApi.listNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apprevisions",
        labelSelector: `zeitwork.com/app=${options.projectId}`,
      })

      // The response structure might vary, so check for items in different locations
      const responseData = response.body || response
      const items = (responseData as any).items || []

      const revisions = items as AppRevision[]
      const deployments: Deployment[] = revisions.map((revision) => ({
        id: revision.metadata?.name || "",
        previewURL: `${revision.spec.commitSHA.substring(0, 7)}-${options.projectId}.zeitwork.app`,
        projectId: options.projectId,
        organisationId: options.organisationId,
      }))

      return { data: deployments, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  return {
    projects: {
      get: getProject,
      list: listProjects,
      create: createProject,
      update: updateProject,
      deploy: deployProject,
    },
    deployments: {
      get: getDeployment,
      list: listDeployments,
    },
    organisations: {
      get: getOrganisation,
      list: listOrganisations,
      create: createOrganisation,
    },
  }
}

// Session management functions
export async function createSession(userId: string): Promise<string> {
  const db = useDrizzle()
  const token = randomBytes(32).toString("hex")
  const expiresAt = addDays(new Date(), 30) // 30 day sessions

  await db.insert(schema.sessions).values({
    userId,
    token,
    expiresAt,
  })

  return token
}

export async function verifySession(token: string): Promise<{ userId: string } | null> {
  const db = useDrizzle()

  const now = new Date()
  const sessions = await db.select().from(schema.sessions).where(eq(schema.sessions.token, token)).limit(1)

  const session = sessions[0]

  if (!session || session.expiresAt < now) {
    return null
  }

  return { userId: session.userId }
}

export async function deleteSession(token: string): Promise<void> {
  const db = useDrizzle()
  await db.delete(schema.sessions).where(eq(schema.sessions.token, token))
}
