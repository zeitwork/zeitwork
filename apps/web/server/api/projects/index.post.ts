import { environmentVariables, githubInstallations, organisations, projects } from "@zeitwork/database/schema";
import { count } from "@zeitwork/database/utils/drizzle";
import z from "zod";
import { encrypt } from "~~/server/utils/crypto";

const bodySchema = z.object({
  name: z.string().min(1).max(255),
  repository: z.object({
    owner: z.string().min(1).max(255),
    repo: z.string().min(1).max(255),
  }),
  secrets: z.array(
    z.object({
      name: z.string().min(1).max(255),
      value: z.string(),
    }),
  ),
  rootDirectory: z
    .string()
    .max(255)
    .regex(/^\/(?:[^./][^/]*(?:\/[^./][^/]*)*)?$/, {
      message: "Root directory must start with / and cannot contain '..' or hidden directories",
    })
    .default("/"),
  dockerfilePath: z
    .string()
    .max(255)
    .regex(/^(?!.*\.\.)(?!\/)[a-zA-Z0-9._\-/]+$/, {
      message: "Dockerfile path must be a relative path without '..' sequences",
    })
    .default("Dockerfile"),
});

export default defineEventHandler(async (event) => {
  const { secure, verified } = await requireVerifiedUser(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });
  if (!verified) throw createError({ statusCode: 403, message: "Account not verified" });

  const body = await readValidatedBody(event, bodySchema.parse);

  // Fetch organisation's project limit
  const [organisation] = await useDrizzle()
    .select({ projectLimit: organisations.projectLimit })
    .from(organisations)
    .where(eq(organisations.id, secure.organisationId));
  if (!organisation) {
    throw createError({ statusCode: 404, message: "Organisation not found" });
  }

  // Enforce project limit
  const [countResult] = await useDrizzle()
    .select({ count: count() })
    .from(projects)
    .where(eq(projects.organisationId, secure.organisationId));
  if (!countResult || countResult.count >= organisation.projectLimit) {
    throw createError({
      statusCode: 403,
      message: `Project limit reached (${organisation.projectLimit} projects maximum)`,
    });
  }

  const githubRepository = `${body.repository.owner}/${body.repository.repo}`;

  // Check if we have access to the GitHub repository and find the githubInstallationId
  const github = useGitHub();

  const installations = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.organisationId, secure.organisationId));

  let githubInstallationId: null | string = null;
  for (const installation of installations) {
    const { data: repo } = await github.repository.get(
      installation.githubInstallationId,
      body.repository.owner,
      body.repository.repo,
    );
    if (repo) {
      githubInstallationId = installation.id;
    }
  }
  if (!githubInstallationId) {
    throw createError({ statusCode: 500, message: "Installation not found" });
  }

  // Create project and environment variables in a transaction
  const { project } = await useDrizzle().transaction(async (tx) => {
    // Create project
    const [project] = await tx
      .insert(projects)
      .values({
        name: body.name,
        slug: slugify(body.name),
        githubRepository: githubRepository,
        githubInstallationId: githubInstallationId,
        organisationId: secure.organisationId,
        rootDirectory: body.rootDirectory,
        dockerfilePath: body.dockerfilePath,
      })
      .returning();
    if (!project) {
      throw createError({ statusCode: 500, message: "Failed to create project" });
    }

    if (body.secrets.length > 0) {
      // Create environment variables with encrypted values
      await tx.insert(environmentVariables).values(
        body.secrets.map((secret) => ({
          name: secret.name,
          value: encrypt(secret.value),
          projectId: project.id,
          organisationId: secure.organisationId,
        })),
      );
    }

    return { project };
  });

  // TODO: Create a deployment for the latest commit

  return {
    project,
  };
});

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-") // replace non-alphanumerics with -
    .replace(/^-+|-+$/g, ""); // trim leading/trailing -
}
