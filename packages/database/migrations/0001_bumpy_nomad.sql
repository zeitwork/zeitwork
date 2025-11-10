ALTER TABLE "vms" ALTER COLUMN "status" SET DATA TYPE text;--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "status" SET DEFAULT 'initializing'::text;--> statement-breakpoint
DROP TYPE "public"."vm_statuses";--> statement-breakpoint
CREATE TYPE "public"."vm_statuses" AS ENUM('initializing', 'starting', 'pooling', 'running', 'deleting', 'unknown');--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "status" SET DEFAULT 'initializing'::"public"."vm_statuses";--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "status" SET DATA TYPE "public"."vm_statuses" USING "status"::"public"."vm_statuses";--> statement-breakpoint
-- drop the no column
ALTER TABLE "vms" DROP COLUMN "no";--> statement-breakpoint
-- add the no column
ALTER TABLE "vms" ADD COLUMN "no" serial NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "server_name" SET NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "server_no" integer NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "server_type" text NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "public_ip" text NOT NULL;--> statement-breakpoint
ALTER TABLE "regions" DROP COLUMN "firewall_no";--> statement-breakpoint
ALTER TABLE "regions" DROP COLUMN "network_no";--> statement-breakpoint
ALTER TABLE "vms" DROP COLUMN "private_ip";--> statement-breakpoint
ALTER TABLE "vms" DROP COLUMN "container_name";--> statement-breakpoint
ALTER TABLE "vms" ADD CONSTRAINT "vms_serverNo_unique" UNIQUE("server_no");