import { useDrizzle, eq } from "../../utils/drizzle"
import * as schema from "~~/packages/database/schema"
import { Webhooks } from "@octokit/webhooks"
import { useZeitworkClient } from "../../utils/api"

export default defineEventHandler(async (event) => {
  console.log(`[${new Date().toISOString()}] [POST] /api/github/webhook`)

  // Get raw body for signature verification
  const rawBody = await readRawBody(event)
  if (!rawBody) {
    console.error(`[${new Date().toISOString()}] ERROR: No body provided`)
    throw createError({
      statusCode: 400,
      statusMessage: "No body provided",
    })
  }
  console.log(`[${new Date().toISOString()}] Raw body received, length: ${rawBody.length}`)

  // Get signature and webhook secret
  const signature = getHeader(event, "x-hub-signature-256")
  const webhookSecret = useRuntimeConfig().githubWebhookSecret

  console.log(`[${new Date().toISOString()}] Signature header: ${signature ? "present" : "missing"}`)
  console.log(`[${new Date().toISOString()}] Webhook secret: ${webhookSecret ? "configured" : "missing"}`)

  if (!signature || !webhookSecret) {
    console.error(`[${new Date().toISOString()}] ERROR: Missing signature or webhook secret`)
    throw createError({
      statusCode: 401,
      statusMessage: "Missing signature or webhook secret",
    })
  }

  // Initialize webhooks with secret
  const webhooks = new Webhooks({
    secret: webhookSecret,
  })

  // Verify signature
  console.log(`[${new Date().toISOString()}] Starting signature verification...`)
  const isValid = await webhooks.verify(rawBody, signature)

  if (!isValid) {
    console.error(`[${new Date().toISOString()}] ERROR: Invalid signature`)
    throw createError({
      statusCode: 401,
      statusMessage: "Invalid signature",
    })
  }
  console.log(`[${new Date().toISOString()}] Signature verified successfully`)

  // Parse the webhook payload
  const payload = JSON.parse(rawBody)
  const eventType = getHeader(event, "x-github-event")

  console.log(`[${new Date().toISOString()}] [POST] received event: ${eventType}`)
  console.log(`[${new Date().toISOString()}] Repository: ${payload.repository?.full_name || "unknown"}`)

  const db = useDrizzle()

  switch (eventType) {
    case "installation":
      // Handle installation created/deleted
      if (payload.action === "created") {
        // Installation was created - user will be redirected to our install endpoint
        console.log("GitHub App installed:", payload.installation.id)
      } else if (payload.action === "deleted") {
        // Remove installation ID from all organisations that have it
        await db
          .update(schema.organisations)
          .set({ installationId: null })
          .where(eq(schema.organisations.installationId, payload.installation.id))

        console.log("GitHub App uninstalled:", payload.installation.id)
      }
      break

    case "installation_repositories":
      // Handle repository access changes
      console.log("Repository access changed:", payload)
      break

    case "push":
      // Handle push events (new commits)
      try {
        // Find the organization by installation ID
        const [organisation] = await db
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.installationId, payload.installation.id))
          .limit(1)

        if (!organisation) {
          console.log("Organisation not found for installation ID:", payload.installation.id)
          return { received: true }
        }

        // Extract repository information from the webhook payload
        const githubOwner = payload.repository.owner.login
        const githubRepo = payload.repository.name
        const commitSHA = payload.after // The SHA of the most recent commit after the push

        // Try to get existing project to check its port
        let port = 3000 // Default port
        const client = useZeitworkClient()

        // The project ID in Kubernetes is based on the GitHub repository ID
        const projectK8sName = `repo-${payload.repository.id}`

        try {
          const { data: existingProject } = await client.projects.get({
            organisationId: organisation.id,
            organisationNo: organisation.no,
            projectId: projectK8sName,
          })

          if (existingProject) {
            useZeitworkClient().projects.deploy({
              organisationId: organisation.id,
              organisationNo: organisation.no,
              projectId: projectK8sName,
              commitSHA,
            })
          }
        } catch (error) {
          // Project doesn't exist yet, will use default port
          console.log(`Project ${projectK8sName} not found, using default port: ${port}`)
        }

        // Create or update the project with the new commit SHA
        const { data, error } = await client.projects.create({
          organisationId: organisation.id,
          name: payload.repository.name, // Use repo name as project name
          githubOwner,
          githubRepo,
          port,
          desiredRevisionSHA: commitSHA,
        })

        if (error) {
          console.error("Failed to create/update project:", error)
        } else {
          console.log(`Push event processed: ${githubOwner}/${githubRepo} with commit ${commitSHA}`)
        }
      } catch (error) {
        console.error("Error handling push event:", error)
      }
      break

    default:
      console.log(`Unhandled webhook event: ${eventType}`)
  }

  // Always return 200 to acknowledge receipt
  return { received: true }
})
