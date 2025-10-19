import {
  integer,
  pgEnum,
  pgTable,
  serial,
  text,
  uuid,
} from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { uuidv7 } from "uuidv7";

export const regions = pgTable("regions", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: serial().notNull().unique(),
  name: text().notNull().unique(), // Hetzner location (e.g., "nbg1", "fsn1", "hel1")
  loadBalancerIpv4: text().notNull(),
  loadBalancerIpv6: text().notNull(),
  loadBalancerNo: integer(),
  firewallNo: integer(),
  networkNo: integer(),
  ...timestamps,
});

export const vmStatuses = pgEnum("vm_statuses", [
  "running",
  "pooling", // the vm is in the pool and waiting to be used
  "initializing",
  "starting",
  "stopping",
  "off",
  "deleting",
  "migrating",
  "rebuilding",
  "unknown",
]);

export const vms = pgTable("vms", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  no: integer().notNull().unique(), // Also serves as Hetzner server ID
  status: vmStatuses().notNull().default("unknown"),
  privateIp: text().notNull(), // Private IP within Hetzner network (no public IPv4 to reduce costs)
  regionId: uuid()
    .notNull()
    .references(() => regions.id),
  imageId: uuid().references(() => images.id),
  port: integer().notNull(),
  serverName: text(),
  containerName: text(),
  ...timestamps,
});

export const images = pgTable("images", {
  id: uuid().primaryKey().$defaultFn(uuidv7),
  registry: text().notNull(), // e.g. docker.io
  repository: text().notNull(), // e.g. library/alpine
  tag: text().notNull(), // e.g. latest
  digest: text().notNull(), // e.g. sha256:1234567890abcdef
  ...timestamps,
});
