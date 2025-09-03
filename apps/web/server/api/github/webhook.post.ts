import { useDeploymentModel } from "~~/server/models/deployment"
import * as schema from "@zeitwork/database/schema"
import { Webhooks } from "@octokit/webhooks"

export default defineEventHandler(async (event) => {
  // Get raw body for signature verification
  const rawBody = await readRawBody(event)
  if (!rawBody) {
    throw createError({
      statusCode: 400,
      statusMessage: "No body provided",
    })
  }

  // Verify webhook signature
  const signature = getHeader(event, "x-hub-signature-256")
  const webhookSecret = useRuntimeConfig().githubWebhookSecret

  if (!signature || !webhookSecret) {
    throw createError({
      statusCode: 401,
      statusMessage: "Missing signature or webhook secret",
    })
  }

  const webhooks = new Webhooks({ secret: webhookSecret })
  const isValid = await webhooks.verify(rawBody, signature)

  if (!isValid) {
    throw createError({
      statusCode: 401,
      statusMessage: "Invalid signature",
    })
  }

  // Parse the webhook payload
  const payload = JSON.parse(rawBody)
  const eventType = getHeader(event, "x-github-event")

  const db = useDrizzle()
  const github = useGitHub()

  switch (eventType) {
    case "installation":
      if (payload.action === "created") {
        // Create a GitHub installation record
        const githubLogin = payload.installation.account.login.toLowerCase()
        const githubAccountId = payload.installation.account.id

        // Look up the organization by slug (which should match the GitHub login)
        const [organisation] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.slug, githubLogin))
          .limit(1)

        if (organisation) {
          // Create or update GitHub installation record
          await db
            .insert(schema.githubInstallations)
            .values({
              githubAccountId: githubAccountId,
              githubInstallationId: payload.installation.id,
              organisationId: organisation.id,
            })
            .onConflictDoUpdate({
              target: [schema.githubInstallations.githubInstallationId],
              set: {
                githubAccountId: githubAccountId,
                organisationId: organisation.id,
              },
            })
        }
      } else if (payload.action === "deleted") {
        // Remove GitHub installation records
        await db
          .delete(schema.githubInstallations)
          .where(eq(schema.githubInstallations.githubInstallationId, payload.installation.id))
      }
      break

    case "installation_repositories":
      break

    case "push":
      try {
        // Find the organization by installation ID through the githubInstallations table
        const [installationRecord] = await db
          .select({
            organisation: schema.organisations,
            installation: schema.githubInstallations,
          })
          .from(schema.githubInstallations)
          .innerJoin(schema.organisations, eq(schema.organisations.id, schema.githubInstallations.organisationId))
          .where(eq(schema.githubInstallations.githubInstallationId, payload.installation.id))
          .limit(1)

        if (!installationRecord) {
          return { received: true }
        }

        const organisation = installationRecord.organisation

        // Extract repository information
        const githubOwner = payload.repository.owner.login
        const githubRepo = payload.repository.name
        const commitSHA = payload.after // The SHA of the most recent commit after the push

        // Optionally get additional repository information using GitHub utility
        const { data: repoInfo, error: repoError } = await github.repository.get(
          payload.installation.id,
          githubOwner,
          githubRepo,
        )

        // Get commit information using GitHub utility
        const { data: commitInfo, error: commitError } = await github.commit.get(
          payload.installation.id,
          githubOwner,
          githubRepo,
          commitSHA,
        )

        // Fetch the project
        const [project] = await db
          .select()
          .from(schema.projects)
          .where(
            and(
              eq(schema.projects.githubRepositoryOwner, githubOwner),
              eq(schema.projects.githubRepositoryName, githubRepo),
            ),
          )
          .limit(1)
        if (!project) {
          throw createError({ statusCode: 400, message: "Project not found" })
        }

        // If branch isn't main, we simply return OK to the webhook
        if (commitSHA !== project.githubDefaultBranch) {
          return { statusCode: 200, statusMessage: "OK" }
        }

        const [productionEnvironment] = await db
          .select()
          .from(schema.projectEnvironments)
          .where(
            and(
              eq(schema.projectEnvironments.projectId, project.id),
              eq(schema.projectEnvironments.name, "production"),
            ),
          )
          .limit(1)
        if (!productionEnvironment) {
          throw createError({ statusCode: 400, message: "Production environment not found" })
        }

        // Insert a new deployment for the project
        const deploymentModel = useDeploymentModel()
        const { data: deployment, error: deploymentError } = await deploymentModel.create({
          projectId: project.id,
          environmentId: productionEnvironment.id,
          organisationId: organisation.id,
        })
        if (deploymentError) {
          throw createError({ statusCode: 500, message: deploymentError.message })
        }
      } catch (error) {
        // Handle error silently for now
      }
      break

    default:
    // Unhandled webhook event
  }

  // For unhandled webhook events
  return { received: true }
})
