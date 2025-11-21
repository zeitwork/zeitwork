import { organisations, organisationMembers } from "@zeitwork/database/schema"
import { eq } from "@zeitwork/database/utils/drizzle"
import type Stripe from "stripe"

export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig()
  const stripe = useStripe()

  // Get raw body for signature verification
  const body = await readRawBody(event, "utf-8")
  const signature = getHeader(event, "stripe-signature")

  console.log("Webhook received:", {
    hasBody: !!body,
    bodyLength: body?.length,
    hasSignature: !!signature,
    webhookSecretConfigured: !!config.stripe.webhookSecret,
    webhookSecretPrefix: config.stripe.webhookSecret?.substring(0, 10),
  })

  if (!body || !signature) {
    throw createError({
      statusCode: 400,
      message: "Missing body or signature",
    })
  }

  let stripeEvent: Stripe.Event

  try {
    // Verify webhook signature
    stripeEvent = stripe.webhooks.constructEvent(body, signature, config.stripe.webhookSecret)
  } catch (err: any) {
    console.error("Webhook signature verification failed:", err.message)
    throw createError({
      statusCode: 400,
      message: `Webhook Error: ${err.message}`,
    })
  }

  // Handle the event
  try {
    switch (stripeEvent.type) {
      case "customer.subscription.created":
      case "customer.subscription.updated":
        await handleSubscriptionUpdate(stripeEvent.data.object as Stripe.Subscription)
        break

      case "customer.subscription.deleted":
        await handleSubscriptionDeleted(stripeEvent.data.object as Stripe.Subscription)
        break

      case "checkout.session.completed":
        await handleCheckoutCompleted(stripeEvent.data.object as Stripe.Checkout.Session)
        break

      default:
        console.log(`Unhandled event type: ${stripeEvent.type}`)
    }
  } catch (err: any) {
    console.error("Error processing webhook:", err)
    throw createError({
      statusCode: 500,
      message: "Error processing webhook",
    })
  }

  return { received: true }
})

async function handleSubscriptionUpdate(subscription: Stripe.Subscription) {
  const customerId = subscription.customer as string

  // Find organization by customer ID
  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.stripeCustomerId, customerId))
    .limit(1)

  if (!organisation) {
    console.error(`Organization not found for customer ${customerId}`)
    return
  }

  const isNewSubscription = !organisation.stripeSubscriptionId

  // Update organization with subscription details
  await useDrizzle()
    .update(organisations)
    .set({
      stripeSubscriptionId: subscription.id,
      stripeSubscriptionStatus: subscription.status,
    })
    .where(eq(organisations.id, organisation.id))

  console.log(`Updated subscription for org ${organisation.id}: ${subscription.status}`)

  // Track subscription events
  const posthog = usePostHog()

  // Find the user associated with this organization to track against their user ID
  const [orgMember] = await useDrizzle()
    .select()
    .from(organisationMembers)
    .where(eq(organisationMembers.organisationId, organisation.id))
    .limit(1)

  if (orgMember) {
    if (isNewSubscription && subscription.status === "active") {
      // Track successful subscription
      posthog.capture({
        distinctId: orgMember.userId.toString(),
        event: "subscription_created",
        properties: {
          organisation_id: organisation.id,
          organisation_slug: organisation.slug,
          subscription_id: subscription.id,
          subscription_status: subscription.status,
          plan_id: subscription.items.data[0]?.price.id,
          plan_amount: subscription.items.data[0]?.price.unit_amount,
          plan_currency: subscription.items.data[0]?.price.currency,
          plan_interval: subscription.items.data[0]?.price.recurring?.interval,
        },
      })
    } else {
      // Track subscription update
      posthog.capture({
        distinctId: orgMember.userId.toString(),
        event: "subscription_updated",
        properties: {
          organisation_id: organisation.id,
          organisation_slug: organisation.slug,
          subscription_id: subscription.id,
          subscription_status: subscription.status,
        },
      })
    }
  }
}

async function handleSubscriptionDeleted(subscription: Stripe.Subscription) {
  const customerId = subscription.customer as string

  // Find organization by customer ID
  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.stripeCustomerId, customerId))
    .limit(1)

  if (!organisation) {
    console.error(`Organization not found for customer ${customerId}`)
    return
  }

  // Update organization subscription status
  await useDrizzle()
    .update(organisations)
    .set({
      stripeSubscriptionStatus: "canceled",
    })
    .where(eq(organisations.id, organisation.id))

  console.log(`Subscription deleted for org ${organisation.id}`)

  // Track subscription cancellation
  const posthog = usePostHog()

  // Find the user associated with this organization to track against their user ID
  const [orgMember] = await useDrizzle()
    .select()
    .from(organisationMembers)
    .where(eq(organisationMembers.organisationId, organisation.id))
    .limit(1)

  if (orgMember) {
    posthog.capture({
      distinctId: orgMember.userId.toString(),
      event: "subscription_canceled",
      properties: {
        organisation_id: organisation.id,
        organisation_slug: organisation.slug,
        subscription_id: subscription.id,
      },
    })
  }
}

async function handleCheckoutCompleted(session: Stripe.Checkout.Session) {
  const organisationId = session.metadata?.organisationId

  if (!organisationId) {
    console.error("No organisation ID in checkout session metadata")
    return
  }

  // The subscription will be updated via subscription.created event
  // This is mainly for logging/analytics
  console.log(`Checkout completed for organisation ${organisationId}`)
}
