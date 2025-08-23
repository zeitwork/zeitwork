import { integer, jsonb, pgTable, text, uuid } from "drizzle-orm/pg-core";
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
  ipAddress: text().notNull(),
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
  ...timestamps,
});
