ALTER TABLE "builds" ADD COLUMN "processing_by" uuid;--> statement-breakpoint
ALTER TABLE "builds" ADD COLUMN "processing_started_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "builds" ADD CONSTRAINT "builds_processing_by_servers_id_fkey" FOREIGN KEY ("processing_by") REFERENCES "servers"("id");