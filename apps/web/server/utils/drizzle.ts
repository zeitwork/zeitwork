import * as schema from "~~/packages/database/schema"
export { sql, eq, and, or, asc, desc } from "drizzle-orm"
import { drizzle } from "drizzle-orm/postgres-js"
import postgres from "postgres"

const client = postgres(process.env.NUXT_DSN!, { prepare: false })

const database = drizzle(client, { schema, casing: "snake_case" })

export function useDrizzle() {
  return database
}
