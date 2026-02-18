export { sql, eq, and, or, asc, desc, isNull } from "@zeitwork/database/utils/drizzle";
import { useClient } from "@zeitwork/database/client";

const client = useClient({ dsn: process.env.NUXT_DSN! });

export function useDrizzle() {
  return client;
}
