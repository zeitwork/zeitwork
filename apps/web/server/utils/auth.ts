import { users } from "@zeitwork/database/schema";
import { eq } from "@zeitwork/database/utils/drizzle";
import type { H3Event } from "h3";

/**
 * Requires a verified user session.
 * - First checks that the user is logged in via requireUserSession
 * - Then queries the database to check if the user is verified (verified_at is set)
 *
 * @returns { secure, verified } - secure is the session data, verified is boolean
 */
export async function requireVerifiedUser(event: H3Event) {
  const { secure } = await requireUserSession(event);

  if (!secure) {
    return { secure: null, verified: false };
  }

  // Query database to check verification status
  const [user] = await useDrizzle()
    .select({ verifiedAt: users.verifiedAt })
    .from(users)
    .where(eq(users.id, secure.userId))
    .limit(1);

  if (!user) {
    return { secure: null, verified: false };
  }

  return { secure, verified: !!user.verifiedAt };
}
