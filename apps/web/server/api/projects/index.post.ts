import { projects, githubInstallations, projectSecrets, projectEnvironments } from "@zeitwork/database/schema"
import { useDeploymentModel } from "~~/server/models/deployment"
import z from "zod"

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
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" })

  const body = await readValidatedBody(event, bodySchema.parse)

  const github = useGitHub()

  // Check if we have access to the repository
  const installations = await useDrizzle()
    .select()
    .from(githubInstallations)
    .where(eq(githubInstallations.organisationId, secure.organisationId))

  let githubInstallationId = null
  let githubRepositoryId = null
  for (const iteration of installations) {
    const { data: repository, error: repositoryError } = await github.repository.get(
      iteration.githubInstallationId,
      body.repository.owner,
      body.repository.repo,
    )
    if (repository) {
      githubInstallationId = iteration.githubInstallationId
      githubRepositoryId = repository.id
      break
    }
  }
  if (!githubInstallationId) {
    throw createError({ statusCode: 400, message: "No installation found for repository" })
  }

  // Check if project already exists
  const [project] = await useDrizzle()
    .select()
    .from(projects)
    .where(
      and(eq(projects.organisationId, secure.organisationId), eq(projects.githubInstallationId, githubInstallationId)),
    )
    .limit(1)
  if (project) {
    throw createError({ statusCode: 400, message: "Project already exists" })
  }

  // Create project and environment variables in a transaction
  const { project: txProject, productionEnv } = await useDrizzle().transaction(async (tx) => {
    // Create project
    const [project] = await tx
      .insert(projects)
      .values({
        name: body.name,
        slug: generateSlug(body.name),
        githubRepository: `${body.repository.owner}/${body.repository.repo}`,
        githubInstallationId: githubInstallationId!,
        defaultBranch: "main",
        organisationId: secure.organisationId,
      })
      .returning()
    if (!project) {
      throw createError({ statusCode: 500, message: "Failed to create project" })
    }

    // Create project environments
    const defaults = ["production", "staging"]
    const envs = await tx
      .insert(projectEnvironments)
      .values(
        defaults.map((env) => ({
          name: env,
          projectId: project.id,
          organisationId: secure.organisationId,
        })),
      )
      .returning()
    if (!envs || envs.length !== defaults.length) {
      throw createError({ statusCode: 500, message: "Failed to create project environments" })
    }

    const productionEnv = envs.find((env) => env.name === "production")
    if (!productionEnv) {
      throw createError({ statusCode: 500, message: "Failed to create production environment" })
    }

    if (body.secrets.length > 0) {
      // Create environment variables
      await tx.insert(projectSecrets).values(
        body.secrets.map((secret) => ({
          name: secret.name,
          value: encryptSecret(secret.value),
          projectId: project.id,
          environmentId: productionEnv.id,
          organisationId: secure.organisationId,
        })),
      )
    }

    return { project, productionEnv }
  })

  // Create a deployment for the latest commit
  const deploymentModel = useDeploymentModel()
  const { data: deployment, error: deploymentError } = await deploymentModel.create({
    projectId: txProject.id,
    environmentId: productionEnv.id,
    organisationId: secure.organisationId,
  })

  if (deploymentError) {
    throw createError({ statusCode: 500, message: deploymentError.message })
  }

  return {
    project: txProject,
    deployment,
  }
})

function generateSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-") // replace non-alphanumerics with -
    .replace(/^-+|-+$/g, "") // trim leading/trailing -
}

function encryptSecret(secret: string): string {
  return secret // TODO: Implement
}

function decryptSecret(secret: string): string {
  return secret // TODO: Implement
}
