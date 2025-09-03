import {
  boolean,
  integer,
  pgTable,
  text,
  timestamp,
  unique,
  uuid,
  type AnyPgColumn,
} from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { images, instances } from "./platform";
import { uuidv7 } from "uuidv7";

const organisationId = {
  organisationId: uuid().references(() => organisations.id),
};

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
  userId: uuid().references(() => users.id),
  githubAccountId: integer().notNull(),
  githubInstallationId: integer().notNull().unique(),
  ...organisationId,
  ...timestamps,
});

export const organisationMembers = pgTable("organisation_members", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  userId: uuid().references(() => users.id),
  ...organisationId,
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

export const projects = pgTable(
  "projects",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(),
    slug: text().notNull(),
    githubRepository: text().notNull(),
    defaultBranch: text().notNull(),
    githubInstallationId: integer().notNull(),
    latestDeploymentId: uuid().references((): AnyPgColumn => deployments.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.slug, t.organisationId)]
);

export const domains = pgTable("domains", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(), // e.g. app.example.com
  verificationToken: text(),
  verifiedAt: timestamp({ withTimezone: true }),
  deploymentId: uuid().references(() => deployments.id),
  internal: boolean().notNull().default(false),
  ...organisationId,
  ...timestamps,
});

export const projectEnvironments = pgTable(
  "project_environments",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(), // production, staging
    projectId: uuid().references(() => projects.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.name, t.projectId, t.organisationId)]
);

export const projectSecrets = pgTable(
  "project_secrets",
  {
    id: uuid().primaryKey().$defaultFn(uuidv7),
    name: text().notNull(),
    value: text().notNull(),
    projectId: uuid().references(() => projects.id),
    environmentId: uuid().references(() => projectEnvironments.id),
    ...organisationId,
    ...timestamps,
  },
  (t) => [unique().on(t.name, t.projectId, t.organisationId)]
);

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: text().unique().notNull(),
  status: text().notNull(), // pending, building, deploying, active, inactive, failed
  commitHash: text().notNull(),
  projectId: uuid().references(() => projects.id),
  environmentId: uuid().references(() => projectEnvironments.id),
  imageId: uuid().references(() => images.id),
  ...organisationId,
  ...timestamps,
});

export const deploymentInstances = pgTable("deployment_instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: uuid().references(() => deployments.id),
  instanceId: uuid().references(() => instances.id),
  ...organisationId,
  ...timestamps,
});

export const imageBuilds = pgTable("image_builds", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: text().notNull(), // pending, building, completed, failed
  deploymentId: uuid().references(() => deployments.id),
  ///
  startedAt: timestamp({ withTimezone: true }),
  completedAt: timestamp({ withTimezone: true }),
  failedAt: timestamp({ withTimezone: true }),
  ///
  ...organisationId,
  ...timestamps,
});
