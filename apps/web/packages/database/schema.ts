import { integer, pgTable, serial, text, timestamp, uuid } from "drizzle-orm/pg-core"
import { uuidv7 } from "uuidv7"

export const waitlist = pgTable("waitlist", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  email: text().notNull().unique(),
  xForwardedFor: text(),
  country: text(),
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
}).enableRLS()

export const users = pgTable("users", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: serial().notNull().unique(),
  name: text().notNull(),
  email: text().notNull().unique(),
  username: text().notNull(),
  githubId: integer().notNull().unique(),
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
}).enableRLS()

export const organisations = pgTable("organisations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: serial().notNull().unique(),
  name: text().notNull(),
  slug: text().notNull().unique(),
  installationId: integer(),
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
}).enableRLS()

export const organisationMembers = pgTable("organisation_members", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: serial().notNull().unique(),
  userId: uuid().references(() => users.id),
  organisationId: uuid().references(() => organisations.id),
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
}).enableRLS()

export const sessions = pgTable("sessions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: serial().notNull().unique(),
  userId: uuid()
    .references(() => users.id)
    .notNull(),
  token: text().notNull().unique(),
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
}).enableRLS()
