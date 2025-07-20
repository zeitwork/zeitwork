import * as k8s from "@kubernetes/client-node"
import { useDrizzle, eq, and } from "./drizzle"
import * as schema from "@zeitwork/database/schema"
import { randomBytes } from "crypto"
import { addDays } from "date-fns"
import { getRepository, getLatestCommitSHA } from "./github"

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
interface AppSpec {
  description: string
  desiredRevision?: string
  fqdn?: string
  githubInstallation: number
  githubOwner: string
  githubRepo: string
  port: number
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
  const kc = new k8s.KubeConfig()
  kc.loadFromDefault()
  return {
    coreV1Api: kc.makeApiClient(k8s.CoreV1Api),
    customObjectsApi: kc.makeApiClient(k8s.CustomObjectsApi),
    config: kc,
  }
}

// Helper function to create or update namespace
async function ensureNamespace(organisationNo: number): Promise<void> {
  const { coreV1Api } = getK8sClient()
  const namespaceName = `tenant-${organisationNo}`

  try {
    await coreV1Api.readNamespace({ name: namespaceName })
    console.log(`Namespace ${namespaceName} already exists`)
  } catch (error: any) {
    if (error.response?.statusCode === 404) {
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
        throw new Error(`Failed to create namespace: ${createError.response?.body?.message || createError.message}`)
      }
    } else {
      console.error(`Failed to check namespace ${namespaceName}:`, error.response?.body || error)
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
    await customObjectsApi.patchNamespacedCustomObject({
      group,
      version,
      namespace,
      plural,
      name: appName,
      body: app,
    } as any) // Note: headers are configured at client level for patch operations
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
      await customObjectsApi.createNamespacedCustomObject({
        group,
        version,
        namespace,
        plural,
        body: app,
      })
    } else {
      console.error(`Failed to check/update app ${appName}:`, error.response?.body || error)
      throw error
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
  }

  async function createProject(options: CreateProjectInput): Promise<ZeitworkResponse<Project>> {
    try {
      const db = useDrizzle()

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
      }

      return { data: project, error: null }
    } catch (error) {
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
