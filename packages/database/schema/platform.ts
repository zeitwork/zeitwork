import {
  integer,
  jsonb,
  pgEnum,
  pgTable,
  text,
  timestamp,
  uuid,
} from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { uuidv7 } from "uuidv7";

export const regions = pgTable("regions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  code: text().notNull().unique(),
  country: text().notNull(),
  ...timestamps,
});

export const nodes = pgTable("nodes", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid()
    .notNull()
    .references(() => regions.id),
  hostname: text().notNull(),
  ipAddress: text().notNull().unique(),
  // internalIpAddress: text().notNull().unique(),
  state: text().notNull(), // booting, ready, draining, down, terminated, error, unknown
  resources: jsonb().notNull(), // { vcpu: 1, memory: 1024 }
  ...timestamps,
});

export const instanceStatuses = pgEnum("instance_statuses", [
  "pending",
  "starting",
  "running",
  "stopping",
  "stopped",
  "failed",
  "terminated",
]);

export const instances = pgTable("instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid()
    .notNull()
    .references(() => regions.id),
  nodeId: uuid()
    .notNull()
    .references(() => nodes.id),
  imageId: uuid()
    .notNull()
    .references(() => images.id),
  state: instanceStatuses().notNull().default("pending"),
  vcpus: integer().notNull(),
  memory: integer().notNull(),
  defaultPort: integer().notNull(),
  ipAddress: text().notNull(), // IP address (IPv4 or IPv6)
  environmentVariables: text().notNull(), // encrypted { key: value } object
  ...timestamps,
});

export const images = pgTable("images", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  size: integer(), // in bytes
  hash: text().notNull(), // sha256 hash of the image
  ...timestamps,
});

export const imageBuildStatus = pgEnum("image_build_status", [
  "pending",
  "building",
  "completed",
  "failed",
]);

// status is computed based on other fields
export const imageBuilds = pgTable("image_builds", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  status: imageBuildStatus().notNull().default("pending"),
  //
  githubRepository: text().notNull(),
  githubCommit: text().notNull(),
  //
  imageId: uuid().references(() => images.id),
  //
  startedAt: timestamp({ withTimezone: true }),
  completedAt: timestamp({ withTimezone: true }),
  failedAt: timestamp({ withTimezone: true }),
  //
  ...timestamps,
});

export const logs = pgTable("logs", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  imageBuildId: uuid().references(() => imageBuilds.id),
  instanceId: uuid().references(() => instances.id),
  level: text(), // optional: "info", "error", "warning", etc.
  message: text().notNull(),
  loggedAt: timestamp({ withTimezone: true }).notNull(),
  ...timestamps,
});
