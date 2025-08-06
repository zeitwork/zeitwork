import { useDrizzle, eq } from "~~/server/utils/drizzle"
import * as schema from "~~/packages/database/schema"
import { Webhooks } from "@octokit/webhooks"
import { useZeitworkClient } from "~~/server/utils/api"

const DEFAULT_PORT = 3000

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

  log(`Received ${eventType} event for ${payload.repository?.full_name || "unknown"}`)

  const db = useDrizzle()
  const client = useZeitworkClient()

  switch (eventType) {
    case "installation":
      if (payload.action === "created") {
        log(`GitHub App installed: ${payload.installation.id}`)
        // 1. look up the organisation, update the organisation with the installation ID
        const githubLogin = payload.installation.account.login.toLowerCase()

        // Look up the organization by slug (which should match the GitHub login)
        const [organisation] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.slug, githubLogin))
          .limit(1)

        if (organisation) {
          // Update the organization with the installation ID
          await db
            .update(schema.organisations)
            .set({ installationId: payload.installation.id })
            .where(eq(schema.organisations.id, organisation.id))

          log(
            `Updated organisation ${organisation.name} (${organisation.slug}) with installation ID ${payload.installation.id}`,
          )
        } else {
          log(`Organisation not found for GitHub account: ${githubLogin}`)
        }
      } else if (payload.action === "deleted") {
        // Remove installation ID from all organisations that have it
        await db
          .update(schema.organisations)
          .set({ installationId: null })
          .where(eq(schema.organisations.installationId, payload.installation.id))

        log(`GitHub App uninstalled: ${payload.installation.id}`)
      }
      break

    case "installation_repositories":
      log(`Repository access changed for installation ${payload.installation.id}`)
      break

    case "push":
      try {
        // Find the organization by installation ID
        const [organisation] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.installationId, payload.installation.id))
          .limit(1)

        if (!organisation) {
          log(`Organisation not found for installation ID: ${payload.installation.id}`)
          return { received: true }
        }

        // Extract repository information
        const githubOwner = payload.repository.owner.login
        const githubRepo = payload.repository.name
        const commitSHA = payload.after // The SHA of the most recent commit after the push
        const projectK8sName = `repo-${payload.repository.id}`

        try {
          // Check if project exists
          const { data: existingProject } = await client.projects.get({
            organisationId: organisation.id,
            organisationNo: organisation.no,
            projectId: projectK8sName,
          })

          if (existingProject) {
            // Deploy the existing project with new commit
            await client.projects.deploy({
              organisationId: organisation.id,
              organisationNo: organisation.no,
              projectId: projectK8sName,
              commitSHA,
            })
            log(`Deployed existing project ${projectK8sName} with commit ${commitSHA}`)
          }
        } catch (error) {
          // Project doesn't exist, create it
          const { data, error: createError } = await client.projects.create({
            organisationId: organisation.id,
            name: payload.repository.name,
            githubOwner,
            githubRepo,
            port: DEFAULT_PORT,
            desiredRevisionSHA: commitSHA,
          })

          if (createError) {
            logError(`Failed to create project ${projectK8sName}`, createError)
          } else {
            log(`Created new project ${projectK8sName} with commit ${commitSHA}`)
          }
        }
      } catch (error) {
        logError("Error handling push event", error)
      }
      break

    default:
      log(`Unhandled webhook event: ${eventType}`)
  }

  // Always return 200 to acknowledge receipt
  return { received: true }
})

// Simple logger helper
function log(message: string) {
  return console.log(`[GitHub Webhook] ${message}`)
}

function logError(message: string, error?: any) {
  console.error(`[GitHub Webhook] ERROR: ${message}`, error || "")
}
