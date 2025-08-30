import { deployments, organisations, projects } from "@zeitwork/database/schema"
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

      let deploymentId = generateDeploymentId()

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
          deploymentId: generateDeploymentId(),
          status: "pending",
          commitHash: latestCommitHash,
          projectId: project.id,
          environmentId: params.environmentId,
        })
        .returning()

      if (!deployment) {
        return { data: null, error: new Error("Failed to create deployment") }
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

const deploymentIdAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const deploymentIdGenerator = customAlphabet(deploymentIdAlphabet, 10)

function generateDeploymentUrl({
  projectId,
  deploymentId,
  organisationSlug,
}: {
  projectId: string
  deploymentId: string
  organisationSlug: string
}) {
  return `${projectId}-${deploymentId}-${organisationSlug}.zeitwork.app`
}

function generateDeploymentId(): string {
  return deploymentIdGenerator()
}
