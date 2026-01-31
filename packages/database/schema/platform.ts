import { integer, pgEnum, pgTable, text, uuid, jsonb } from "drizzle-orm/pg-core";
import { timestamps } from "../utils/timestamps";
import { uuidv7 } from "uuidv7";

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
  imageId: uuid().references(() => images.id),
  port: integer(),
  ipAddress: text(),
  metadata: jsonb(), // { pid: 1234 }
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
