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

// export const ipv6Allocations = pgTable("ipv6_allocations", {
//   id: uuid().primaryKey().$defaultFn(uuidv7),
//   regionId: uuid().references(() => regions.id),
//   nodeId: uuid().references(() => nodes.id),
//   instanceId: uuid().references(() => instances.id),
//   ipv6Address: text().notNull().unique(), // Full IPv6 address
//   prefix: text().notNull(), // IPv6 prefix for this allocation
//   state: text().notNull(), // allocated, released, reserved
//   ...timestamps,
// });

export const instances = pgTable("instances", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  regionId: uuid().references(() => regions.id),
  nodeId: uuid().references(() => nodes.id),
  imageId: uuid().references(() => images.id),
  state: text().notNull(), // pending, starting, running, stopping, stopped, failed, terminated
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
  objectKey: text(), // S3 object key
  ...timestamps,
});

export const sslCerts = pgTable("ssl_certs", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  key: text().notNull().unique(), // Certificate key path (e.g., "certificates/acme/example.com/example.com.crt")
  value: text().notNull(), // Certificate data (base64 encoded)
  expiresAt: timestamp({ withTimezone: true }),
  ...timestamps,
});

export const sslLocks = pgTable("ssl_locks", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  key: text().notNull().unique(), // Lock key
  expiresAt: timestamp({ withTimezone: true }).notNull(),
  ...timestamps,
});
