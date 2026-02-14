import { projects, githubInstallations } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import { z } from "zod";

const paramsSchema = z.object({
  id: z.string(),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const { id } = await getValidatedRouterParams(event, paramsSchema.parse);

  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(and(eq(projects.slug, id), eq(projects.organisationId, secure.organisationId)))
    .limit(1);
  if (!project) {
    throw createError({ statusCode: 404, message: "Project not found" });
  }

  // Look up the GitHub installation record
  const [installation] = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.id, project.githubInstallationId))
    .limit(1);

  if (!installation) {
    return {
      connected: false,
      repository: project.githubRepository,
      error: "GitHub installation record not found",
    };
  }

  // Check if the installation is still valid on GitHub
  const github = useGitHub();
  const { data: octokit, error: octokitError } = await github.installation.getOctokit(
    installation.githubInstallationId,
  );

  if (octokitError) {
    return {
      connected: false,
      repository: project.githubRepository,
      error: "GitHub installation is no longer valid",
    };
  }

  // Verify the installation can still access the repository
  const [owner, repo] = project.githubRepository.split("/");
  const { data: repoData, error: repoError } = await github.repository.get(
    installation.githubInstallationId,
    owner,
    repo,
  );

  if (repoError || !repoData) {
    return {
      connected: false,
      repository: project.githubRepository,
      error: "GitHub installation cannot access the repository",
    };
  }

  return {
    connected: true,
    repository: project.githubRepository,
    error: null,
  };
});
