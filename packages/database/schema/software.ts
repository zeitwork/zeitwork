import { integer, pgTable, text, timestamp, uuid } from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { images, instances } from "./platform";
import { uuidv7 } from "uuidv7";

export const waitlist = pgTable("waitlist", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  email: text().notNull().unique(),
  ...timestamps,
});

export const users = pgTable("users", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  email: text().notNull().unique(),
  username: text().notNull(),
  githubUserId: integer(),
  ...timestamps,
});

export const organisations = pgTable("organisations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  ...timestamps,
});

export const githubInstallations = pgTable("github_installations", {
  id: integer().primaryKey(),
  githubInstallationId: integer().notNull(),
  githubOrgId: integer().notNull(),
  organisationId: uuid().references(() => organisations.id),
  userId: uuid().references(() => users.id),
  ...timestamps,
});

export const organisationMembers = pgTable("organisation_members", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid().references(() => users.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const sessions = pgTable("sessions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid()
    .references(() => users.id)
    .notNull(),
  token: text().notNull().unique(),
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  ...timestamps,
});

export const projects = pgTable("projects", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const projectDomains = pgTable("project_domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  domainId: uuid().references(() => domains.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const domains = pgTable("domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  verificationToken: text(),
  verifiedAt: timestamp({ withTimezone: true }),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const domainRecords = pgTable("domain_records", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  domainId: uuid().references(() => domains.id),
  type: text().notNull(),
  name: text().notNull(),
  content: text().notNull(),
  ttl: integer().notNull(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const projectEnvironments = pgTable("project_environments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  name: text().notNull(), // production, staging
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const projectSecrets = pgTable("project_secrets", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  name: text().notNull(),
  value: text().notNull(),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid().references(() => projects.id),
  projectEnvironmentId: uuid().references(() => projectEnvironments.id),
  status: text().notNull(), // pending, building, deploying, active, inactive, failed
  commitHash: text().notNull(),
  imageId: uuid().references(() => images.id),
  organisationId: uuid().references(() => organisations.id),
  deploymentUrl: text(), // project-nanoid-org.zeitwork.app
  nanoid: text().unique(), // Unique deployment identifier
  rolloutStrategy: text().notNull().default("blue-green"), // blue-green, canary, rolling
  minInstances: integer().notNull().default(3), // Minimum instances per region
  activatedAt: timestamp({ withTimezone: true }),
  deactivatedAt: timestamp({ withTimezone: true }),
  ...timestamps,
});

export const deploymentInstances = pgTable("deployment_instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: uuid().references(() => deployments.id),
  instanceId: uuid().references(() => instances.id),
  organisationId: uuid().references(() => organisations.id),
  ...timestamps,
});
