import * as schema from "./schema";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";

export function useClient({ dsn }: { dsn: string }) {
  const conn = postgres(dsn, { prepare: false });

  return drizzle({
    client: conn,
    schema,
    casing: "snake_case",
  });
}
