import { useDrizzle, eq } from "../../utils/drizzle"
import * as schema from "~~/packages/database/schema"

export default defineEventHandler(async (event) => {
  // GitHub will redirect here with:
  // - installation_id: The ID of the installation
  // - setup_action: 'install' or 'update'
  // - state: Optional state parameter to maintain context

  const query = getQuery(event)
  const { installation_id, setup_action, state } = query

  if (!installation_id) {
    throw createError({
      statusCode: 400,
      statusMessage: "Missing installation_id",
    })
  }

  // Get the current user session
  const { user } = await requireUserSession(event)
  if (!user) {
    // Store installation_id in cookie and redirect to login
    setCookie(event, "pending_installation", installation_id as string)
    return sendRedirect(event, "/login")
  }

  const db = useDrizzle()

  try {
    // If state contains an org ID, update that specific org
    // Otherwise, update the user's default org
    let orgSlug = (state as string) || user.username

    const [org] = await db
      .update(schema.organisations)
      .set({
        installationId: parseInt(installation_id as string),
      })
      .where(eq(schema.organisations.slug, orgSlug))
      .returning()

    if (!org) {
      throw createError({
        statusCode: 404,
        statusMessage: "Organisation not found",
      })
    }

    // Redirect to org page or settings with success message
    return sendRedirect(event, `/${orgSlug}?installed=true`)
  } catch (error) {
    console.error("Failed to save installation:", error)
    throw createError({
      statusCode: 500,
      statusMessage: "Failed to save installation",
    })
  }
})
