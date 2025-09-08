import { deployments, organisations, projects, domains } from "@zeitwork/database/schema"
import { customAlphabet } from "nanoid"
import { eq } from "../utils/drizzle"

type ModelResponse<T> =
  | {
      data: T
      error: null
    }
  | {
      data: null
      error: Error
    }

export function useDeploymentModel() {
  interface CreateDeploymentParams {
    projectId: string
    environmentId: string
    organisationId: string
  }

  async function createDeployment(
    params: CreateDeploymentParams,
  ): Promise<ModelResponse<typeof deployments.$inferSelect | null>> {
    try {
      const github = useGitHub()

      // Get the project
      const [project] = await useDrizzle().select().from(projects).where(eq(projects.id, params.projectId)).limit(1)

      if (!project) {
        return { data: null, error: new Error("Project not found") }
      }

      const [organisation] = await useDrizzle()
        .select()
        .from(organisations)
        .where(eq(organisations.id, params.organisationId))
        .limit(1)

      if (!organisation) {
        return { data: null, error: new Error("Organisation not found") }
      }

      const deploymentId = generateDeploymentId()

      const { data: latestCommitHash, error: latestCommitHashError } = await github.branch.getLatestCommitSHA(
        project.githubInstallationId,
        project.githubRepository.split("/")[0],
        project.githubRepository.split("/")[1],
        project.defaultBranch,
      )
      if (latestCommitHashError) {
        return { data: null, error: new Error("Failed to get latest commit hash") }
      }

      const [deployment] = await useDrizzle()
        .insert(deployments)
        .values({
          deploymentId: deploymentId,
          status: "pending",
          commitHash: latestCommitHash,
          projectId: project.id,
          environmentId: params.environmentId,
          organisationId: params.organisationId,
        })
        .returning()

      if (!deployment) {
        return { data: null, error: new Error("Failed to create deployment") }
      }

      // Create internal domain for the deployment
      const internalDomainName = generateInternalDomain(project.slug, deploymentId, organisation.slug)

      try {
        await useDrizzle().insert(domains).values({
          name: internalDomainName,
          deploymentId: deployment.id,
          internal: true,
          verifiedAt: new Date(), // Internal domains are always verified
          organisationId: params.organisationId,
        })
      } catch (domainError) {
        // Log the error but don't fail the deployment creation
        console.error("Failed to create internal domain:", domainError)
      }

      return { data: deployment, error: null }
    } catch (error) {
      return { data: null, error: error instanceof Error ? error : new Error("Unknown error") }
    }
  }

  return {
    create: createDeployment,
  }
}

const deploymentIdAlphabet = "123456789abcdefghijkmnopqrstuvwxyz"
const deploymentIdGenerator = customAlphabet(deploymentIdAlphabet, 10)

function generateDeploymentId(): string {
  return deploymentIdGenerator()
}

/**
 * Generates an internal domain name for a deployment
 * Pattern: <project-slug>-<deployment-id>-<org-slug>.zeitwork.app (production)
 * Pattern: <project-slug>-<deployment-id>-<org-slug>.zeitwork.localhost (development)
 */
function generateInternalDomain(projectSlug: string, deploymentId: string, orgSlug: string): string {
  const isDevelopment = process.env.NODE_ENV === "development" || process.env.ENVIRONMENT === "development"
  const baseDomain = isDevelopment ? "zeitwork.localhost" : "zeitwork.app"
  return `${projectSlug}-${deploymentId}-${orgSlug}.${baseDomain}`
}
