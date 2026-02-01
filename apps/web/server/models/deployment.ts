import {
  deployments,
  organisations,
  projects,
  domains,
  githubInstallations,
} from "@zeitwork/database/schema";
import { eq } from "../utils/drizzle";
import { nanoid } from "nanoid";

type ModelResponse<T> =
  | {
      data: T;
      error: null;
    }
  | {
      data: null;
      error: Error;
    };

export function useDeploymentModel() {
  interface CreateDeploymentParams {
    projectId: string;
    organisationId: string;
  }

  async function createDeployment(
    params: CreateDeploymentParams,
  ): Promise<ModelResponse<typeof deployments.$inferSelect | null>> {
    try {
      const github = useGitHub();

      // Get the project
      const [project] = await useDrizzle()
        .select()
        .from(projects)
        .where(eq(projects.id, params.projectId))
        .limit(1);
      if (!project) {
        return { data: null, error: new Error("Project not found") };
      }

      const [organisation] = await useDrizzle()
        .select()
        .from(organisations)
        .where(eq(organisations.id, params.organisationId))
        .limit(1);
      if (!organisation) {
        return { data: null, error: new Error("Organisation not found") };
      }

      const [githubInstallation] = await useDrizzle()
        .select()
        .from(githubInstallations)
        .where(eq(githubInstallations.id, project.githubInstallationId))
        .limit(1);
      if (!githubInstallation) {
        return { data: null, error: new Error("GitHub installation not found") };
      }

      const { data: latestCommitHash, error: latestCommitHashError } =
        await github.branch.getLatestCommitSHA(
          githubInstallation.githubInstallationId,
          project.githubRepository.split("/")[0],
          project.githubRepository.split("/")[1],
          "main",
        );
      if (latestCommitHashError) {
        return { data: null, error: new Error("Failed to get latest commit hash") };
      }

      const [deployment] = await useDrizzle()
        .insert(deployments)
        .values({
          status: "pending",
          projectId: project.id,
          githubCommit: latestCommitHash,
          organisationId: params.organisationId,
        })
        .returning();

      if (!deployment) {
        return { data: null, error: new Error("Failed to create deployment") };
      }

      // Create internal domain for the deployment
      const internalDomainName = generateInternalDomain(
        project.slug,
        organisation.slug,
      );

      try {
        await useDrizzle().insert(domains).values({
          name: internalDomainName,
          projectId: project.id,
          deploymentId: deployment.id,
          organisationId: params.organisationId,
          verifiedAt: new Date(),
        });
      } catch (domainError) {
        // Log the error but don't fail the deployment creation
        console.error("Failed to create internal domain:", domainError);
      }

      return { data: deployment, error: null };
    } catch (error) {
      return { data: null, error: error instanceof Error ? error : new Error("Unknown error") };
    }
  }

  return {
    create: createDeployment,
  };
}

/**
 * Generates an internal domain name for a deployment
 * Pattern: <project-slug>-<nanoid>-<org-slug>.zeitwork.app
 */
function generateInternalDomain(projectSlug: string, orgSlug: string): string {
  const id = nanoid(6);
  return `${projectSlug}-${id}-${orgSlug}.zeitwork.app`.toLowerCase();
}
