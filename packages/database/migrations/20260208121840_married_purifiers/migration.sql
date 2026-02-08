CREATE TYPE "server_status" AS ENUM('active', 'draining', 'drained', 'dead');--> statement-breakpoint
CREATE TABLE "servers" (
	"id" uuid PRIMARY KEY,
	"hostname" text NOT NULL,
	"internal_ip" text NOT NULL,
	"ip_range" cidr NOT NULL,
	"status" "server_status" DEFAULT 'active'::"server_status" NOT NULL,
	"last_heartbeat_at" timestamp with time zone DEFAULT now() NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "server_id" uuid;--> statement-breakpoint
ALTER TABLE "vms" ADD CONSTRAINT "vms_server_id_servers_id_fkey" FOREIGN KEY ("server_id") REFERENCES "servers"("id");