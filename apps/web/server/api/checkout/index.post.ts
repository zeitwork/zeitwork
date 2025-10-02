import { organisations } from "@zeitwork/database/schema"
import { eq } from "drizzle-orm"
import { z } from "zod"

const bodySchema = z.object({
  priceId: z.string(),
  returnUrl: z.string().url().optional(),
})

export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event)

  if (!secure?.organisationId) {
    throw createError({
      statusCode: 401,
      message: "Unauthorized",
    })
  }

  const body = await readValidatedBody(event, bodySchema.parse)
  const config = useRuntimeConfig()
  const stripe = useStripe()

  // Get the organization
  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.id, secure.organisationId!))
    .limit(1)

  if (!organisation) {
    throw createError({
      statusCode: 404,
      message: "Organization not found",
    })
  }

  let customerId = organisation.stripeCustomerId

  // Create Stripe customer if doesn't exist
  if (!customerId) {
    const customer = await stripe.customers.create({
      metadata: {
        organisationId: organisation.id,
        organisationSlug: organisation.slug,
      },
    })

    customerId = customer.id

    // Update organization with customer ID
    await useDrizzle()
      .update(organisations)
      .set({ stripeCustomerId: customerId })
      .where(eq(organisations.id, organisation.id))
  }

  // Create checkout session
  const session = await stripe.checkout.sessions.create({
    customer: customerId,
    mode: "subscription",
    payment_method_types: ["card"],
    line_items: [
      {
        price: body.priceId,
        quantity: 1,
      },
    ],
    success_url: body.returnUrl || `${config.appUrl}/${organisation.slug}?checkout=success`,
    cancel_url: body.returnUrl || `${config.appUrl}/${organisation.slug}?checkout=cancelled`,
    metadata: {
      organisationId: organisation.id,
    },
  })

  return {
    sessionId: session.id,
    url: session.url,
  }
})
