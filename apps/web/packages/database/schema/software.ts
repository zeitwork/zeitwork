import { integer, pgTable, serial, text, timestamp, uuid } from "drizzle-orm/pg-core"
import { uuidv7 } from "uuidv7"
import { instances } from "./platform"

// Helpers
const timestamps = {
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  deletedAt: timestamp({ withTimezone: true }),
}

export const waitlist = pgTable("waitlist", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  email: text().notNull().unique(),
  ...timestamps,
})

export const users = pgTable("users", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  email: text().notNull().unique(),
  username: text().notNull(),
  githubId: integer(),
  githubInstallationId: integer(), // TODO
  ...timestamps,
})

export const organisations = pgTable("organisations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  installationId: integer(), // TODO
  ...timestamps,
})

export const organisationMembers = pgTable("organisation_members", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid().references(() => users.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const sessions = pgTable("sessions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid()
    .references(() => users.id)
    .notNull(),
  token: text().notNull().unique(),
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  ...timestamps,
})

export const projects = pgTable("projects", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const projectDomains = pgTable("project_domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  domainId: uuid().references(() => domains.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const domains = pgTable("domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  verificationToken: text(),
  verifiedAt: timestamp({ withTimezone: true }),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const domainRecords = pgTable("domain_records", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  domainId: uuid().references(() => domains.id),
  type: text().notNull(),
  name: text().notNull(),
  content: text().notNull(),
  ttl: integer().notNull(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const projectEnvironments = pgTable("project_environments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  name: text().notNull(), // production, staging
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const projectSecrets = pgTable("project_secrets", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  name: text().notNull(),
  value: text().notNull(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  projectEnvironmentId: uuid().references(() => projectEnvironments.id),
  status: text().notNull(),
  commitHash: text().notNull(),
  branch: text().notNull(),
  message: text().notNull(),
  buildFailedAt: timestamp({ withTimezone: true }),
  buildSuccessAt: timestamp({ withTimezone: true }),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})

export const deploymentInstances = pgTable("deployment_instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: uuid().references(() => deployments.id),
  instanceId: uuid().references(() => instances.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
})
