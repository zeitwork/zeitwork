import { useDrizzle, eq } from "../../utils/drizzle"
import * as schema from "@zeitwork/database/schema"
import crypto from "crypto"

// Verify GitHub webhook signature
function verifyWebhookSignature(payload: string, signature: string, secret: string): boolean {
  const hmac = crypto.createHmac("sha256", secret)
  const digest = "sha256=" + hmac.update(payload).digest("hex")
  return crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(digest))
}

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
  const webhookSecret = process.env.GITHUB_WEBHOOK_SECRET

  if (!signature || !webhookSecret) {
    throw createError({
      statusCode: 401,
      statusMessage: "Missing signature or webhook secret",
    })
  }

  if (!verifyWebhookSignature(rawBody, signature, webhookSecret)) {
    throw createError({
      statusCode: 401,
      statusMessage: "Invalid signature",
    })
  }

  // Parse the webhook payload
  const payload = JSON.parse(rawBody)
  const eventType = getHeader(event, "x-github-event")

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

    default:
      console.log(`Unhandled webhook event: ${eventType}`)
  }

  // Always return 200 to acknowledge receipt
  return { received: true }
})
