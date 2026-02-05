import { organisations, organisationMembers } from "@zeitwork/database/schema";
import { eq, and } from "@zeitwork/database/utils/drizzle";
import type { H3Event } from "h3";

/**
 * Valid Stripe subscription statuses that allow access to the product
 */
const VALID_SUBSCRIPTION_STATUSES = ["active", "trialing"];

/**
 * Checks if an organization has a valid, paid subscription
 */
export async function hasValidSubscription(organisationId: string): Promise<boolean> {
  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.id, organisationId))
    .limit(1);

  if (!organisation) {
    return false;
  }

  // Check if organization has a subscription with valid status
  if (
    organisation.stripeSubscriptionId &&
    organisation.stripeSubscriptionStatus &&
    VALID_SUBSCRIPTION_STATUSES.includes(organisation.stripeSubscriptionStatus)
  ) {
    return true;
  }

  return false;
}

/**
 * Require that the user's organization has a valid subscription
 * Throws a 403 error if not subscribed
 */
export async function requireSubscription(event: H3Event) {
  const { secure } = await requireUserSession(event);

  if (!secure?.organisationId) {
    throw createError({
      statusCode: 401,
      message: "Unauthorized",
    });
  }

  const isSubscribed = await hasValidSubscription(secure.organisationId);

  if (!isSubscribed) {
    throw createError({
      statusCode: 403,
      message: "Active subscription required",
    });
  }

  return { organisationId: secure.organisationId, userId: secure.userId };
}

/**
 * Get the first organisation for a user (used as default)
 */
export async function getFirstUserOrganisation(userId: string) {
  const [member] = await useDrizzle()
    .select()
    .from(organisationMembers)
    .where(eq(organisationMembers.userId, userId))
    .limit(1);

  if (!member) {
    return null;
  }

  const [organisation] = await useDrizzle()
    .select()
    .from(organisations)
    .where(eq(organisations.id, member.organisationId))
    .limit(1);

  return organisation || null;
}
