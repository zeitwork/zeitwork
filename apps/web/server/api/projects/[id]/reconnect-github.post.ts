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

  // Load all GitHub installations for this organisation
  const installations = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.organisationId, secure.organisationId));

  if (installations.length === 0) {
    throw createError({
      statusCode: 400,
      message: "No GitHub App installations found. Please install the GitHub App first.",
    });
  }

  // Check which installation can access the project's repository
  const github = useGitHub();
  const [owner, repo] = project.githubRepository.split("/");

  let matchingInstallationId: string | null = null;
  for (const installation of installations) {
    const { data: repoData } = await github.repository.get(
      installation.githubInstallationId,
      owner,
      repo,
    );
    if (repoData) {
      matchingInstallationId = installation.id;
      break;
    }
  }

  if (!matchingInstallationId) {
    throw createError({
      statusCode: 400,
      message:
        "No GitHub App installation has access to this repository. Please install the GitHub App on the account that owns this repository.",
    });
  }

  // Update the project to point to the valid installation
  const [updatedProject] = await useDrizzle()
    .update(projects)
    .set({
      githubInstallationId: matchingInstallationId,
      updatedAt: new Date(),
    })
    .where(eq(projects.id, project.id))
    .returning();

  return { success: true, project: updatedProject };
});
