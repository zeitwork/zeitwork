import { useDeploymentModel } from "~~/server/models/deployment"
import * as schema from "@zeitwork/database/schema"
import { Webhooks } from "@octokit/webhooks"

export default defineEventHandler(async (event) => {
  const eventType = getHeader(event, "x-github-event")
  const deliveryId = getHeader(event, "x-github-delivery")

  try {
    // Get raw body for signature verification
    const rawBody = await readRawBody(event)
    if (!rawBody) {
      console.error(`[GitHub Webhook] No body provided (event: ${eventType}, delivery: ${deliveryId})`)
      throw createError({
        statusCode: 400,
        statusMessage: "No body provided",
      })
    }

    // Verify webhook signature
    const signature = getHeader(event, "x-hub-signature-256")
    const webhookSecret = useRuntimeConfig().githubWebhookSecret

    if (!signature) {
      console.error(`[GitHub Webhook] Missing signature (event: ${eventType}, delivery: ${deliveryId})`)
      throw createError({
        statusCode: 401,
        statusMessage: "Missing signature",
      })
    }

    if (!webhookSecret) {
      console.error(
        `[GitHub Webhook] Missing webhook secret configuration (event: ${eventType}, delivery: ${deliveryId})`,
      )
      throw createError({
        statusCode: 500,
        statusMessage: "Webhook secret not configured",
      })
    }

    // Verify the webhook signature
    const webhooks = new Webhooks({ secret: webhookSecret })
    let isValid = false
    try {
      isValid = await webhooks.verify(rawBody, signature)
    } catch (error: any) {
      console.error(
        `[GitHub Webhook] Signature verification error (event: ${eventType}, delivery: ${deliveryId}):`,
        error,
      )
      throw createError({
        statusCode: 401,
        statusMessage: "Signature verification failed",
      })
    }

    if (!isValid) {
      console.error(`[GitHub Webhook] Invalid signature (event: ${eventType}, delivery: ${deliveryId})`)
      throw createError({
        statusCode: 401,
        statusMessage: "Invalid signature",
      })
    }

    // Parse the webhook payload
    let payload
    try {
      payload = JSON.parse(rawBody)
    } catch (error: any) {
      console.error(`[GitHub Webhook] Failed to parse payload (event: ${eventType}, delivery: ${deliveryId}):`, error)
      throw createError({
        statusCode: 400,
        statusMessage: "Invalid JSON payload",
      })
    }

    console.log(`[GitHub Webhook] Received ${eventType} event (delivery: ${deliveryId})`)

    const db = useDrizzle()
    const github = useGitHub()

    switch (eventType) {
      case "installation":
        try {
          if (payload.action === "created") {
            const githubLogin = payload.installation?.account?.login?.toLowerCase()
            const githubAccountId = payload.installation?.account?.id
            const installationId = payload.installation?.id

            if (!githubLogin || !githubAccountId || !installationId) {
              console.error(
                `[GitHub Webhook - installation] Missing required fields in payload (delivery: ${deliveryId})`,
              )
              return { received: true, error: "Missing required fields" }
            }

            console.log(
              `[GitHub Webhook - installation] Creating installation for ${githubLogin} (ID: ${installationId})`,
            )

            // Look up the organization by slug
            const [organisation] = await db
              .select()
              .from(schema.organisations)
              .where(eq(schema.organisations.slug, githubLogin))
              .limit(1)

            if (organisation) {
              await db
                .insert(schema.githubInstallations)
                .values({
                  githubAccountId: githubAccountId,
                  githubInstallationId: installationId,
                  organisationId: organisation.id,
                })
                .onConflictDoUpdate({
                  target: [schema.githubInstallations.githubInstallationId],
                  set: {
                    githubAccountId: githubAccountId,
                    organisationId: organisation.id,
                  },
                })

              console.log(`[GitHub Webhook - installation] Successfully created/updated installation ${installationId}`)
            } else {
              console.warn(`[GitHub Webhook - installation] Organisation not found for slug: ${githubLogin}`)
            }
          } else if (payload.action === "deleted") {
            const installationId = payload.installation?.id

            if (!installationId) {
              console.error(
                `[GitHub Webhook - installation] Missing installation ID in delete payload (delivery: ${deliveryId})`,
              )
              return { received: true, error: "Missing installation ID" }
            }

            console.log(`[GitHub Webhook - installation] Deleting installation ${installationId}`)

            await db
              .delete(schema.githubInstallations)
              .where(eq(schema.githubInstallations.githubInstallationId, installationId))

            console.log(`[GitHub Webhook - installation] Successfully deleted installation ${installationId}`)
          }
        } catch (error: any) {
          console.error(
            `[GitHub Webhook - installation] Error processing installation event (delivery: ${deliveryId}):`,
            error,
          )
          // Don't throw - return success to avoid webhook retries
          return { received: true, error: error.message }
        }
        break

      case "installation_repositories":
        // Acknowledge but don't process
        console.log(`[GitHub Webhook - installation_repositories] Event acknowledged (delivery: ${deliveryId})`)
        break

      case "push":
        try {
          const installationId = payload.installation?.id
          const githubOwner = payload.repository?.owner?.login
          const githubRepo = payload.repository?.name
          const commitSHA = payload.after
          const ref = payload.ref

          if (!installationId || !githubOwner || !githubRepo || !commitSHA) {
            console.error(`[GitHub Webhook - push] Missing required fields (delivery: ${deliveryId})`)
            return { received: true, error: "Missing required fields" }
          }

          console.log(
            `[GitHub Webhook - push] Processing push to ${githubOwner}/${githubRepo} (commit: ${commitSHA.substring(0, 7)})`,
          )

          // Find the organization by installation ID
          const [installationRecord] = await db
            .select({
              organisation: schema.organisations,
              installation: schema.githubInstallations,
            })
            .from(schema.githubInstallations)
            .innerJoin(schema.organisations, eq(schema.organisations.id, schema.githubInstallations.organisationId))
            .where(eq(schema.githubInstallations.githubInstallationId, installationId))
            .limit(1)

          if (!installationRecord) {
            console.warn(
              `[GitHub Webhook - push] Installation ${installationId} not found in database (delivery: ${deliveryId})`,
            )
            return { received: true }
          }

          const organisation = installationRecord.organisation

          // Fetch repository info (log errors but don't fail)
          const { data: repoInfo, error: repoError } = await github.repository.get(
            installationId,
            githubOwner,
            githubRepo,
          )

          if (repoError) {
            console.error(`[GitHub Webhook - push] Failed to fetch repo info (delivery: ${deliveryId}):`, repoError)
          }

          // Fetch commit info (log errors but don't fail)
          const { data: commitInfo, error: commitError } = await github.commit.get(
            installationId,
            githubOwner,
            githubRepo,
            commitSHA,
          )

          if (commitError) {
            console.error(`[GitHub Webhook - push] Failed to fetch commit info (delivery: ${deliveryId}):`, commitError)
          }

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
            console.warn(
              `[GitHub Webhook - push] Project not found for ${githubOwner}/${githubRepo} (delivery: ${deliveryId})`,
            )
            return { received: true, message: "Project not found" }
          }

          // Check if this is the default branch
          const branchName = ref?.replace("refs/heads/", "")
          if (branchName !== project.githubDefaultBranch) {
            console.log(
              `[GitHub Webhook - push] Ignoring push to non-default branch ${branchName} (delivery: ${deliveryId})`,
            )
            return { received: true, message: "Non-default branch" }
          }

          // Find production environment
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
            console.error(
              `[GitHub Webhook - push] Production environment not found for project ${project.id} (delivery: ${deliveryId})`,
            )
            return { received: true, error: "Production environment not found" }
          }

          // Create deployment
          const deploymentModel = useDeploymentModel()
          const { data: deployment, error: deploymentError } = await deploymentModel.create({
            projectId: project.id,
            environmentId: productionEnvironment.id,
            organisationId: organisation.id,
          })

          if (deploymentError) {
            console.error(
              `[GitHub Webhook - push] Failed to create deployment (delivery: ${deliveryId}):`,
              deploymentError,
            )
            return { received: true, error: deploymentError.message }
          }

          console.log(
            `[GitHub Webhook - push] Successfully created deployment ${deployment?.id} (delivery: ${deliveryId})`,
          )
        } catch (error: any) {
          console.error(`[GitHub Webhook - push] Error processing push event (delivery: ${deliveryId}):`, error)
          // Don't throw - return success to avoid webhook retries
          return { received: true, error: error.message }
        }
        break

      default:
        console.log(`[GitHub Webhook] Unhandled event type: ${eventType} (delivery: ${deliveryId})`)
    }

    return { received: true }
  } catch (error: any) {
    // If it's already an H3Error, rethrow it
    if (error.statusCode) {
      throw error
    }

    console.error(`[GitHub Webhook] Unexpected error (event: ${eventType}, delivery: ${deliveryId}):`, error)
    throw createError({
      statusCode: 500,
      statusMessage: "Internal server error processing webhook",
    })
  }
})
