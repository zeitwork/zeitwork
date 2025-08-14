import { integer, pgTable, text, timestamp, uuid } from "drizzle-orm/pg-core"
import { uuidv7 } from "uuidv7"

// Helpers
const timestamps = {
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  deletedAt: timestamp({ withTimezone: true }),
}

export const regions = pgTable("regions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  code: text().notNull().unique(),
  country: text().notNull(),
  ...timestamps,
})

export const nodes = pgTable("nodes", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid().references(() => regions.id),
  hostname: text().notNull().unique(),
  capacity: integer().notNull(),
  state: text().notNull(), // ready, draining, down, terminated
  ...timestamps,
})

export const instances = pgTable("instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid().references(() => regions.id),
  nodeId: uuid().references(() => nodes.id),
  state: text().notNull(), // pending, starting, running, stopping, stopped, failed, terminated
  imageRef: text().notNull(),
  ipAddress: text().notNull(),
  ...timestamps,
})
