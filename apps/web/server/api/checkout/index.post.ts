import { organisations } from "@zeitwork/database/schema"
import { eq } from "@zeitwork/database/utils/drizzle"
import { z } from "zod"

const bodySchema = z.object({
  priceId: z.string(),
  returnUrl: z.url().optional(),
})

export default defineEventHandler(async (event) => {
  try {
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
      try {
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
      } catch (error) {
        console.error(`[Checkout] Failed to create Stripe customer:`, error)
        throw error
      }
    }

    // Create checkout session
    try {
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

      // Track checkout session created
      const posthog = usePostHog()
      posthog.capture({
        distinctId: secure.userId.toString(),
        event: "checkout_session_created",
        properties: {
          organisation_id: organisation.id,
          organisation_slug: organisation.slug,
          price_id: body.priceId,
          session_id: session.id,
          customer_id: customerId,
        },
      })

      return {
        sessionId: session.id,
        url: session.url,
      }
    } catch (error: any) {
      // Handle case where customer ID exists in DB but not in Stripe
      if (error?.code === "resource_missing" && error?.param === "customer") {
        try {
          // Create new customer
          const customer = await stripe.customers.create({
            metadata: {
              organisationId: organisation.id,
              organisationSlug: organisation.slug,
            },
          })

          customerId = customer.id

          // Update organization with new customer ID
          await useDrizzle()
            .update(organisations)
            .set({ stripeCustomerId: customerId })
            .where(eq(organisations.id, organisation.id))

          // Retry checkout session creation with new customer
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

          // Track checkout session created
          const posthog = usePostHog()
          posthog.capture({
            distinctId: secure.userId.toString(),
            event: "checkout_session_created",
            properties: {
              organisation_id: organisation.id,
              organisation_slug: organisation.slug,
              price_id: body.priceId,
              session_id: session.id,
              customer_id: customerId,
              customer_recreated: true,
            },
          })

          return {
            sessionId: session.id,
            url: session.url,
          }
        } catch (retryError) {
          console.error(`[Checkout] Failed to recover from missing customer:`, retryError)
          throw retryError
        }
      }

      console.error(`[Checkout] Failed to create checkout session:`, error)
      throw error
    }
  } catch (error) {
    // Re-throw if it's already a createError
    if (error && typeof error === "object" && "statusCode" in error) {
      throw error
    }

    // Handle validation errors
    if (error instanceof z.ZodError) {
      throw createError({
        statusCode: 400,
        message: "Invalid request body",
        data: error.errors,
      })
    }

    // Generic error response
    throw createError({
      statusCode: 500,
      message: "Failed to create checkout session",
    })
  }
})
