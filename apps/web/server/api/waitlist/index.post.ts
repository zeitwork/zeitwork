import { waitlist } from "~~/packages/database/schema"
import { count, gte, eq } from "drizzle-orm"
import { subDays } from "date-fns"
import { z } from "zod"

const bodySchema = z.object({
  email: z.string().email(),
})

export default defineEventHandler(async (event) => {
  const { data, success } = await readValidatedBody(event, bodySchema.safeParse)
  if (!success) return sendError(event, createError({ statusCode: 400, statusMessage: "Invalid request body" }))

  // const ip = getRequestHeader(event, "x-forwarded-for")
  // const now = new Date()

  // if (process.env.NODE_ENV !== "development") {
  //   // If either ip or country is not set, return 400
  //   if (!ip) return sendError(event, createError({ statusCode: 400, statusMessage: "Invalid request body" }))

  //   // Rate limit
  //   try {
  //     const [rateLimit] = await useDrizzle()
  //       .select({ count: count() })
  //       .from(waitlist)
  //       .where(and(eq(waitlist.xForwardedFor, ip), gte(waitlist.createdAt, subDays(now, 1))))

  //     if (rateLimit && rateLimit.count > 10) {
  //       return sendError(event, createError({ statusCode: 429, statusMessage: "Rate limit exceeded" }))
  //     }
  //   } catch {
  //     return sendError(event, createError({ statusCode: 500, statusMessage: "Internal server error" }))
  //   }
  // }

  // Upsert waitlist
  try {
    await useDrizzle()
      .insert(waitlist)
      .values({
        email: data.email,
        // xForwardedFor: ip,
      })
      .onConflictDoNothing({ target: waitlist.email })
  } catch (error) {
    console.error(error)
    return sendError(event, createError({ statusCode: 500, statusMessage: "Internal server error" }))
  }

  return {
    success: true,
  }
})
