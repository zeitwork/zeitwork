import {
  boolean,
  integer,
  jsonb,
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
  regionId: uuid().references(() => regions.id),
  hostname: text().notNull().unique(),
  ipAddress: text().notNull(),
  state: text().notNull(), // booting, ready, draining, down, terminated, error, unknown
  resources: jsonb().notNull(), // { vcpu: 1, memory: 1024 }
  ...timestamps,
});

export const instances = pgTable("instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid().references(() => regions.id),
  nodeId: uuid().references(() => nodes.id),
  imageId: uuid().references(() => images.id),
  state: text().notNull(), // pending, starting, running, stopping, stopped, failed, terminated
  resources: jsonb().notNull(), // { vcpu: 1, memory: 1024 }
  defaultPort: integer().notNull(),
  ipAddress: text().notNull(), // ipv6 address
  environmentVariables: text().notNull(), // { key: "value" }
  ...timestamps,
});

export const images = pgTable("images", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  name: text().notNull(),
  status: text().notNull(), // building, ready, failed
  repository: jsonb().notNull(), // { owner: "owner", repo: "repo", branch: "branch", commit: "commit" },
  imageSize: integer(), // in bytes
  imageHash: text().notNull(), // sha256 hash of the image
  s3Bucket: text(), // S3 bucket name
  s3Key: text(), // S3 object key
  builderNodeId: uuid().references(() => nodes.id), // Node that built the image
  ...timestamps,
});

export const routingCache = pgTable("routing_cache", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  domain: text().notNull().unique(),
  deploymentId: uuid(), // References deployments from software.ts
  instances: jsonb().notNull(), // Array of { instanceId, ipAddress, regionCode, healthy }
  version: integer().notNull().default(1),
  ...timestamps,
});

export const ipv6Allocations = pgTable("ipv6_allocations", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid().references(() => regions.id),
  nodeId: uuid().references(() => nodes.id),
  instanceId: uuid().references(() => instances.id),
  ipv6Address: text().notNull().unique(), // Full IPv6 address
  prefix: text().notNull(), // IPv6 prefix for this allocation
  state: text().notNull(), // allocated, released, reserved
  ...timestamps,
});

export const tlsCertificates = pgTable("tls_certificates", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  domain: text().notNull(),
  certificate: text().notNull(), // PEM encoded certificate
  privateKey: text().notNull(), // PEM encoded private key (encrypted)
  issuer: text().notNull(), // Let's Encrypt, ZeroSSL, etc.
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  autoRenew: boolean().notNull().default(true),
  ...timestamps,
});

export const buildQueue = pgTable("build_queue", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  projectId: uuid(), // References projects from software.ts
  imageId: uuid().references(() => images.id),
  priority: integer().notNull().default(0),
  status: text().notNull(), // pending, processing, completed, failed
  githubRepo: text().notNull(),
  commitHash: text().notNull(),
  branch: text().notNull(),
  buildStartedAt: timestamp({ withTimezone: true }),
  buildCompletedAt: timestamp({ withTimezone: true }),
  buildLog: text(),
  ...timestamps,
});
