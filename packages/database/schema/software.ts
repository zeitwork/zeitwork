import { integer, pgEnum, pgTable, text, timestamp, unique, uuid } from "drizzle-orm/pg-core";
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
  "running",
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

export const buildStatusEnum = pgEnum("build_status", ["pending", "building", "success", "error"]);

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

// Certificate storage for certmagic
export const certmagic = pgTable("certmagic", {
  key: text().primaryKey(), // e.g., "acme/example.com/sites/example.com/example.com.crt"
  value: text().notNull(), // bytea in Postgres, stores certificates/keys as binary
  modified: timestamp({ withTimezone: true }).notNull().defaultNow(),
});

export const certmagicLocks = pgTable("certmagic_locks", {
  key: text().primaryKey(), // lock name
  expires: timestamp({ withTimezone: true }).notNull().defaultNow(),
});
