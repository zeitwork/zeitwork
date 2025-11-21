import * as schema from "../schema";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";

export function useClient({ dsn }: { dsn: string }) {
  return drizzle(postgres(dsn, { prepare: false }), {
    schema: schema,
    casing: "snake_case",
  });
}
