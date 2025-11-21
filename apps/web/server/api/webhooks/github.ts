import { useDeploymentModel } from "~~/server/models/deployment"
import { tryCatch } from "~~/server/utils/tryCatch"
import * as schema from "@zeitwork/database/schema"
import { Webhooks } from "@octokit/webhooks"

export default defineEventHandler(async (event) => {
  const eventType = getHeader(event, "x-github-event")
  const deliveryId = getHeader(event, "x-github-delivery")

  try {
    const { data: rawBody, error: rawBodyError } = await tryCatch(readRawBody(event))
    if (rawBodyError || !rawBody) {
      console.error(`[GitHub Webhook] No body provided (event: ${eventType}, delivery: ${deliveryId})`)
      throw createError({
        statusCode: 400,
        statusMessage: "No body provided",
      })
    }

    await verifySignature(event, rawBody, eventType, deliveryId)

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

    const db = useDrizzle()
    const github = useGitHub()

    switch (eventType) {
      case "installation":
        return await handleInstallationEvent(payload, db, deliveryId)

      case "push":
        return await handlePushEvent(payload, db, github, deliveryId)

      case "installation_repositories":
        // Acknowledge but don't process
        break

      default:
      // Unhandled event type
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

async function verifySignature(event: any, rawBody: string, eventType?: string, deliveryId?: string) {
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

  const webhooks = new Webhooks({ secret: webhookSecret })

  const { data: isValid, error: verifyError } = await tryCatch(webhooks.verify(rawBody, signature))

  if (verifyError) {
    console.error(
      `[GitHub Webhook] Signature verification error (event: ${eventType}, delivery: ${deliveryId}):`,
      verifyError,
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
}

async function handleInstallationEvent(payload: any, db: any, deliveryId?: string) {
  if (payload.action === "created") {
    const githubLogin = payload.installation?.account?.login?.toLowerCase()
    const githubAccountId = payload.installation?.account?.id
    const installationId = payload.installation?.id

    if (!githubLogin || !githubAccountId || !installationId) {
      console.error(`[GitHub Webhook - installation] Missing required fields in payload (delivery: ${deliveryId})`)
      return { received: true, error: "Missing required fields" }
    }

    // Look up the organization by slug
    const { data: organisationsList, error: findOrgError } = await tryCatch<any[]>(
      db.select().from(schema.organisations).where(eq(schema.organisations.slug, githubLogin)).limit(1),
    )

    if (findOrgError) {
      console.error(
        `[GitHub Webhook - installation] Error finding organisation (delivery: ${deliveryId}):`,
        findOrgError,
      )
      return { received: true, error: findOrgError.message }
    }

    const organisation = organisationsList?.[0]

    if (organisation) {
      const { error: insertError } = await tryCatch(
        db
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
          }),
      )

      if (insertError) {
        console.error(
          `[GitHub Webhook - installation] Error inserting installation (delivery: ${deliveryId}):`,
          insertError,
        )
        return { received: true, error: insertError.message }
      }
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

    const { error: deleteError } = await tryCatch(
      db.delete(schema.githubInstallations).where(eq(schema.githubInstallations.githubInstallationId, installationId)),
    )

    if (deleteError) {
      console.error(
        `[GitHub Webhook - installation] Error deleting installation (delivery: ${deliveryId}):`,
        deleteError,
      )
      return { received: true, error: deleteError.message }
    }
  }
  return { received: true }
}

async function handlePushEvent(payload: any, db: any, github: any, deliveryId?: string) {
  const installationId = payload.installation?.id
  const githubOwner = payload.repository?.owner?.login
  const githubRepo = payload.repository?.name
  const commitSHA = payload.after
  const ref = payload.ref

  if (!installationId || !githubOwner || !githubRepo || !commitSHA) {
    console.error(`[GitHub Webhook - push] Missing required fields (delivery: ${deliveryId})`)
    return { received: true, error: "Missing required fields" }
  }

  // Find the organization by installation ID
  const { data: installationRecords, error: findInstallationError } = await tryCatch<any[]>(
    db
      .select({
        organisation: schema.organisations,
        installation: schema.githubInstallations,
      })
      .from(schema.githubInstallations)
      .innerJoin(schema.organisations, eq(schema.organisations.id, schema.githubInstallations.organisationId))
      .where(eq(schema.githubInstallations.githubInstallationId, installationId))
      .limit(1),
  )

  if (findInstallationError) {
    console.error(
      `[GitHub Webhook - push] Error finding installation (delivery: ${deliveryId}):`,
      findInstallationError,
    )
    return { received: true, error: findInstallationError.message }
  }

  const installationRecord = installationRecords?.[0]

  if (!installationRecord) {
    console.warn(
      `[GitHub Webhook - push] Installation ${installationId} not found in database (delivery: ${deliveryId})`,
    )
    return { received: true }
  }

  const organisation = installationRecord.organisation

  // Fetch repository info (log errors but don't fail)
  const { error: repoError } = await github.repository.get(installationId, githubOwner, githubRepo)

  if (repoError) {
    console.error(`[GitHub Webhook - push] Failed to fetch repo info (delivery: ${deliveryId}):`, repoError)
  }

  // Fetch commit info (log errors but don't fail)
  const { error: commitError } = await github.commit.get(installationId, githubOwner, githubRepo, commitSHA)

  if (commitError) {
    console.error(`[GitHub Webhook - push] Failed to fetch commit info (delivery: ${deliveryId}):`, commitError)
  }

  // Fetch the project using githubRepository field
  const githubRepository = `${githubOwner}/${githubRepo}`
  const { data: projectsList, error: findProjectError } = await tryCatch<any[]>(
    db.select().from(schema.projects).where(eq(schema.projects.githubRepository, githubRepository)).limit(1),
  )

  if (findProjectError) {
    console.error(`[GitHub Webhook - push] Error finding project (delivery: ${deliveryId}):`, findProjectError)
    return { received: true, error: findProjectError.message }
  }

  const project = projectsList?.[0]

  if (!project) {
    console.warn(`[GitHub Webhook - push] Project not found for ${githubRepository} (delivery: ${deliveryId})`)
    return { received: true, message: "Project not found" }
  }

  // Find environment matching the branch
  const branchName = ref?.replace("refs/heads/", "")
  const { data: environments, error: findEnvError } = await tryCatch<any[]>(
    db
      .select()
      .from(schema.projectEnvironments)
      .where(
        and(eq(schema.projectEnvironments.projectId, project.id), eq(schema.projectEnvironments.branch, branchName)),
      )
      .limit(1),
  )

  if (findEnvError) {
    console.error(`[GitHub Webhook - push] Error finding environment (delivery: ${deliveryId}):`, findEnvError)
    return { received: true, error: findEnvError.message }
  }

  const environment = environments?.[0]

  if (!environment) {
    console.log(
      `[GitHub Webhook - push] No environment found for branch ${branchName} in project ${project.id} (delivery: ${deliveryId})`,
    )
    return { received: true, message: "No matching environment" }
  }

  // Create deployment
  const deploymentModel = useDeploymentModel()
  const { error: deploymentError } = await deploymentModel.create({
    projectId: project.id,
    environmentId: environment.id,
    organisationId: organisation.id,
  })

  if (deploymentError) {
    console.error(`[GitHub Webhook - push] Failed to create deployment (delivery: ${deliveryId}):`, deploymentError)
    return { received: true, error: deploymentError.message }
  }

  return { received: true }
}
