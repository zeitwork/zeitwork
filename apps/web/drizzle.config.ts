import { defineConfig } from "drizzle-kit"

export default defineConfig({
  dialect: "postgresql",
  schema: "./packages/database/schema.ts",
  out: "./packages/database/migrations",
  dbCredentials: { url: process.env.NUXT_DSN! },
  casing: "snake_case",
})
