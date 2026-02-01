import { sql } from "drizzle-orm";
import {
  inet,
  integer,
  jsonb,
  pgEnum,
  pgTable,
  text,
  timestamp,
  unique,
  uuid,
} from "drizzle-orm/pg-core";
import { uuidv7 } from "uuidv7";

// Helpers
export const timestamps = {
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
  deletedAt: timestamp({ withTimezone: true }),
};

export const organisationId = {
  organisationId: uuid()
    .notNull()
    .references(() => organisations.id),
};

// Tables
export const users = pgTable("users", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  email: text().notNull().unique(),
  username: text().notNull().unique(),
  profilePictureUrl: text(),
  githubAccountId: integer(),
  ...timestamps,
});

export const organisations = pgTable("organisations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  slug: text().notNull().unique(),
  ...timestamps,
});

export const githubInstallations = pgTable("github_installations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid()
    .notNull()
    .references(() => users.id),
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

export const domains = pgTable(
  "domains",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(), // e.g. app.example.com
    projectId: uuid()
      .notNull()
      .references(() => projects.id),
    deploymentId: uuid().references(() => deployments.id),
    verifiedAt: timestamp({ withTimezone: true }),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.name, t.projectId)],
);

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
  (t) => [unique().on(t.slug, t.organisationId)],
);

export const environmentVariables = pgTable(
  "environment_variables",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(),
    value: text().notNull(),
    projectId: uuid()
      .notNull()
      .references(() => projects.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.name, t.projectId)],
);

export const deploymentStatusEnum = pgEnum("deployment_status", [
  "pending",
  "building",
  "starting",
  "running",
  "stopping",
  "stopped",
  "failed",
]);

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: deploymentStatusEnum().notNull().default("pending"),
  githubCommit: text().notNull(),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  buildId: uuid().references(() => builds.id),
  imageId: uuid().references(() => images.id),
  vmId: uuid().references(() => vms.id),
  //
  pendingAt: timestamp({ withTimezone: true }),
  buildingAt: timestamp({ withTimezone: true }),
  startingAt: timestamp({ withTimezone: true }),
  runningAt: timestamp({ withTimezone: true }),
  stoppingAt: timestamp({ withTimezone: true }),
  stoppedAt: timestamp({ withTimezone: true }),
  failedAt: timestamp({ withTimezone: true }),
  //
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

export const buildStatusEnum = pgEnum("build_status", [
  "pending",
  "building",
  "succesful",
  "failed",
]);

export const builds = pgTable("builds", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: buildStatusEnum().notNull().default("pending"),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  githubCommit: text().notNull(),
  githubBranch: text().notNull(),
  imageId: uuid().references(() => images.id),
  vmId: uuid().references(() => vms.id),
  //
  pendingAt: timestamp({ withTimezone: true }),
  buildingAt: timestamp({ withTimezone: true }),
  successfulAt: timestamp({ withTimezone: true }),
  failedAt: timestamp({ withTimezone: true }),
  //
  ...organisationId,
  ...timestamps,
});

export const buildLogs = pgTable("build_logs", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  buildId: uuid()
    .notNull()
    .references(() => builds.id),
  message: text().notNull(),
  level: text().notNull(),
  ...organisationId,
  createdAt: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

export const images = pgTable("images", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  // input
  registry: text().notNull(), // e.g. docker.io
  repository: text().notNull(), // e.g. library/alpine
  tag: text().notNull(), // e.g. latest
  // digest: text().notNull(), // e.g. sha256:1234567890abcdef
  // output
  diskImageKey: text(), // if this is null we haven't created the disk image yet
  //
  ...timestamps,
});

// Certificate storage for certmagic
export const certmagicData = pgTable("certmagic_data", {
  key: text().primaryKey(), // e.g., "acme/example.com/sites/example.com/example.com.crt"
  value: text().notNull(), // bytea in Postgres, stores certificates/keys as binary
  modified: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

export const certmagicLocks = pgTable("certmagic_locks", {
  key: text().primaryKey(), // lock name
  expires: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

// PLATFORM

export const vmStatusEnum = pgEnum("vm_status", [
  "pending",
  "starting",
  "running",
  "stopping",
  "stopped",
  "failed",
]);

export const vms = pgTable("vms", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  vcpus: integer().notNull(),
  memory: integer().notNull(),
  status: vmStatusEnum().notNull(),
  imageId: uuid()
    .references(() => images.id)
    .notNull(),
  port: integer(),
  ipAddress: inet().notNull(),
  metadata: jsonb(), // { pid: 1234 }
  //
  pendingAt: timestamp({ withTimezone: true }),
  startingAt: timestamp({ withTimezone: true }),
  runningAt: timestamp({ withTimezone: true }),
  stoppingAt: timestamp({ withTimezone: true }),
  stoppedAt: timestamp({ withTimezone: true }),
  failedAt: timestamp({ withTimezone: true }),
  //
  ...timestamps,
});
// NOTE: ADD CONSTRAINT exclude_overlapping_networks EXCLUDE USING gist (ip_address inet_ops WITH &&);
