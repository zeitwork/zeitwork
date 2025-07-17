import * as k8s from "@kubernetes/client-node"
import { useDrizzle, eq, and } from "./drizzle"
import * as schema from "@zeitwork/database/schema"

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
async function ensureNamespace(organisationId: string): Promise<void> {
  const { coreV1Api } = getK8sClient()
  const namespaceName = `org-${organisationId.toLowerCase()}`

  try {
    await coreV1Api.readNamespace({ name: namespaceName })
  } catch (error: any) {
    if (error.response?.statusCode === 404) {
      // Namespace doesn't exist, create it
      const namespace: k8s.V1Namespace = {
        metadata: {
          name: namespaceName,
          labels: {
            "zeitwork.com/organisation-id": organisationId,
          },
        },
      }
      await coreV1Api.createNamespace({ body: namespace })
    } else {
      throw error
    }
  }
}

// Helper to create or update App custom resource
async function createOrUpdateApp(
  namespace: string,
  appName: string,
  organisationId: string,
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
        "zeitwork.com/organisationId": organisationId.toString(),
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
    if (error.response?.statusCode === 404) {
      // App doesn't exist, create it
      await customObjectsApi.createNamespacedCustomObject({
        group,
        version,
        namespace,
        plural,
        body: app,
      })
    } else {
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
          name: organisation.name,
          slug: organisation.slug,
        },
        error: null,
      }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface GetOrganisationOptions {
    organisationId: string
    userId: string
  }

  async function getOrganisation(options: GetOrganisationOptions): Promise<ZeitworkResponse<Organisation>> {
    try {
      const db = useDrizzle()

      // Check if user has access to this organisation
      const [memberRecord] = await db
        .select()
        .from(schema.organisationMembers)
        .where(
          and(
            eq(schema.organisationMembers.userId, options.userId),
            eq(schema.organisationMembers.organisationId, options.organisationId),
          ),
        )
        .limit(1)

      if (!memberRecord) {
        return { data: null, error: new Error("Organisation not found or access denied") }
      }

      // Get the organisation
      const [organisation] = await db
        .select()
        .from(schema.organisations)
        .where(eq(schema.organisations.id, options.organisationId))
        .limit(1)

      if (!organisation) {
        return { data: null, error: new Error("Organisation not found") }
      }

      return {
        data: {
          id: organisation.id,
          name: organisation.name,
          slug: organisation.slug,
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
          name: schema.organisations.name,
          slug: schema.organisations.slug,
        })
        .from(schema.organisations)
        .innerJoin(schema.organisationMembers, eq(schema.organisations.id, schema.organisationMembers.organisationId))
        .where(eq(schema.organisationMembers.userId, options.userId))

      return {
        data: organisations,
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
  }

  interface Project {
    id: string
    k8sName: string
    name: string
    organisationId: string
    githubOwner: string
    githubRepo: string
    port: number
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

      // Ensure namespace exists for the organisation
      await ensureNamespace(organisation.id)

      const namespace = `org-${organisation.id.toLowerCase()}`
      const appName = `repo-${options.githubRepo.replace(/[^a-z0-9-]/gi, "-").toLowerCase()}`

      // Create or update the app
      await createOrUpdateApp(namespace, appName, organisation.id, {
        description: options.name,
        desiredRevision: options.desiredRevisionSHA,
        githubInstallation: organisation.installationId,
        githubOwner: options.githubOwner,
        githubRepo: options.githubRepo,
        port: options.port,
      })

      const project: Project = {
        id: appName,
        k8sName: appName,
        name: options.name,
        organisationId: options.organisationId,
        githubOwner: options.githubOwner,
        githubRepo: options.githubRepo,
        port: options.port,
      }

      return { data: project, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface ListProjectsOptions {
    organisationId: string
  }

  async function listProjects(options: ListProjectsOptions): Promise<ZeitworkResponse<Project[]>> {
    try {
      const { customObjectsApi, coreV1Api } = getK8sClient()
      const namespace = `org-${options.organisationId.toLowerCase()}`

      // Check if namespace exists
      try {
        await coreV1Api.readNamespace({ name: namespace })
      } catch (error: any) {
        if (error.response?.statusCode === 404) {
          // Namespace doesn't exist, return empty list
          return { data: [], error: null }
        }
        throw error
      }

      // List apps in the namespace
      const response = await customObjectsApi.listNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
      })

      const apps = (response.body as any).items as App[]
      const projects: Project[] = apps.map((app) => ({
        id: app.metadata?.name || "",
        k8sName: app.metadata?.name || "",
        name: app.spec.description,
        organisationId: options.organisationId,
        githubOwner: app.spec.githubOwner,
        githubRepo: app.spec.githubRepo,
        port: app.spec.port,
      }))

      return { data: projects, error: null }
    } catch (error) {
      return { data: null, error: error as Error }
    }
  }

  interface GetProjectOptions {
    organisationId: string
    projectId: string
  }

  async function getProject(options: GetProjectOptions): Promise<ZeitworkResponse<Project>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `org-${options.organisationId.toLowerCase()}`

      const response = await customObjectsApi.getNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apps",
        name: options.projectId,
      })

      const app = response.body as App
      const project: Project = {
        id: app.metadata?.name || "",
        k8sName: app.metadata?.name || "",
        name: app.spec.description,
        organisationId: options.organisationId,
        githubOwner: app.spec.githubOwner,
        githubRepo: app.spec.githubRepo,
        port: app.spec.port,
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
  }

  async function getDeployment(options: GetDeploymentOptions): Promise<ZeitworkResponse<Deployment>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `org-${options.organisationId.toLowerCase()}`

      const response = await customObjectsApi.getNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apprevisions",
        name: options.deploymentId,
      })

      const revision = response.body as AppRevision
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
  }

  async function listDeployments(options: ListDeploymentsOptions): Promise<ZeitworkResponse<Deployment[]>> {
    try {
      const { customObjectsApi } = getK8sClient()
      const namespace = `org-${options.organisationId.toLowerCase()}`

      // List app revisions with label selector
      const response = await customObjectsApi.listNamespacedCustomObject({
        group: "zeitwork.com",
        version: "v1alpha1",
        namespace,
        plural: "apprevisions",
        labelSelector: `zeitwork.com/app=${options.projectId}`,
      })

      const revisions = (response.body as any).items as AppRevision[]
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
