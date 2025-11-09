import {
  boolean,
  integer,
  pgEnum,
  pgTable,
  text,
  timestamp,
  unique,
  uuid,
} from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { images, vms } from "./platform";
import { uuidv7 } from "uuidv7";

const organisationId = {
  organisationId: uuid()
    .notNull()
    .references(() => organisations.id),
};

export const users = pgTable("users", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  email: text().notNull().unique(),
  username: text().notNull(),
  githubAccountId: integer(),
  ...timestamps,
});

export const organisations = pgTable("organisations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  stripeCustomerId: text(),
  stripeSubscriptionId: text(),
  stripeSubscriptionStatus: text(),
  ...timestamps,
});

export const githubInstallations = pgTable("github_installations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid().references(() => users.id),
  githubAccountId: integer().notNull(),
  githubInstallationId: integer().notNull().unique(),
  ...organisationId,
  ...timestamps,
});

export const organisationMembers = pgTable("organisation_members", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid()
    .notNull()
    .references(() => users.id),
  ...organisationId,
  ...timestamps,
});

export const sessions = pgTable("sessions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid()
    .notNull()
    .references(() => users.id),
  token: text().notNull().unique(),
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  ...timestamps,
});

export const sslCertificateStatusesEnum = pgEnum("ssl_certificate_statuses", [
  "pending",
  "active",
  "failed",
  "renewing",
]);

export const domains = pgTable("domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(), // e.g. app.example.com
  deploymentId: uuid().references(() => deployments.id),
  verificationToken: text(),
  verifiedAt: timestamp({ withTimezone: true }),
  sslCertificateStatus: sslCertificateStatusesEnum().default("pending"),
  sslCertificateIssuedAt: timestamp({ withTimezone: true }),
  sslCertificateExpiresAt: timestamp({ withTimezone: true }),
  sslCertificateError: text(),
  ...organisationId,
  ...timestamps,
});

export const projects = pgTable(
  "projects",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(),
    slug: text().notNull(),
    githubRepository: text().notNull(),
    githubInstallationId: uuid()
      .notNull()
      .references(() => githubInstallations.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.slug, t.organisationId)]
);

export const projectEnvironments = pgTable(
  "project_environments",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(), // production, staging
    branch: text().notNull(), // main, develop, etc.
    projectId: uuid()
      .notNull()
      .references(() => projects.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.name, t.projectId, t.organisationId)]
);

export const environmentDomains = pgTable("environment_domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  domainId: uuid()
    .notNull()
    .references(() => domains.id),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  environmentId: uuid()
    .notNull()
    .references(() => projectEnvironments.id),
  ...organisationId,
  ...timestamps,
});

export const environmentVariables = pgTable(
  "environment_variables",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(),
    value: text().notNull(),
    projectId: uuid()
      .notNull()
      .references(() => projects.id),
    environmentId: uuid()
      .notNull()
      .references(() => projectEnvironments.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [
    unique("environment_variables_name_unique").on(
      t.name,
      t.projectId,
      t.environmentId,
      t.organisationId
    ),
  ]
);

export const deploymentStatusesEnum = pgEnum("deployment_statuses", [
  "queued",
  "building",
  "ready",
  "inactive",
  "failed",
]);

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: deploymentStatusesEnum().notNull().default("queued"),
  deploymentId: text().unique().notNull(),
  githubCommit: text().notNull(),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  environmentId: uuid()
    .notNull()
    .references(() => projectEnvironments.id),
  buildId: uuid().references(() => builds.id),
  imageId: uuid().references(() => images.id),
  vmId: uuid().references(() => vms.id),
  ...organisationId,
  ...timestamps,
});

export const deploymentLogs = pgTable("deployment_logs", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: uuid()
    .notNull()
    .references(() => deployments.id),
  message: text().notNull(),
  level: text(),
  ...organisationId,
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

export const buildStatusesEnum = pgEnum("build_statuses", [
  "queued",
  "initializing",
  "building",
  "ready",
  "canceled",
  "error",
]);

// status is computed based on other fields
export const builds = pgTable("builds", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: buildStatusesEnum().notNull().default("queued"),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  githubCommit: text().notNull(),
  githubBranch: text().notNull(),
  imageId: uuid().references(() => images.id),
  vmId: uuid().references(() => vms.id),
  ...organisationId,
  ...timestamps,
});

export const buildLogs = pgTable("build_logs", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  buildId: uuid()
    .notNull()
    .references(() => builds.id),
  message: text().notNull(),
  level: text(),
  ...organisationId,
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

// Certificate storage for certmagic (compatible with certmagic-sql library)
export const certmagicData = pgTable("certmagic_data", {
  key: text().primaryKey(), // e.g., "acme/example.com/sites/example.com/example.com.crt"
  value: text().notNull(), // bytea in Postgres, stores certificates/keys as binary
  modified: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

export const certmagicLocks = pgTable("certmagic_locks", {
  key: text().primaryKey(), // lock name
  expires: timestamp({ withTimezone: true }).notNull().defaultNow(),
});
