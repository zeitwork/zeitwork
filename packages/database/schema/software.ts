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
import { imageBuilds, images, instances } from "./platform";
import { uuidv7 } from "uuidv7";

const organisationId = {
  organisationId: uuid()
    .notNull()
    .references(() => organisations.id),
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
    // latestDeploymentId: uuid().references((): AnyPgColumn => deployments.id),
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

export const deploymentStatuses = pgEnum("deployment_statuses", [
  "pending",
  "building",
  "deploying",
  "active",
  "inactive",
  "failed",
]);

export const deployments = pgTable("deployments", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: text().unique().notNull(),
  status: deploymentStatuses().notNull().default("pending"),
  githubCommit: text().notNull(),
  projectId: uuid()
    .notNull()
    .references(() => projects.id),
  environmentId: uuid()
    .notNull()
    .references(() => projectEnvironments.id),
  imageId: uuid().references(() => images.id),
  imageBuildId: uuid().references(() => imageBuilds.id),
  ...organisationId,
  ...timestamps,
});

export const deploymentInstances = pgTable("deployment_instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  deploymentId: uuid()
    .notNull()
    .references(() => deployments.id),
  instanceId: uuid()
    .notNull()
    .references(() => instances.id),
  ...organisationId,
  ...timestamps,
});
