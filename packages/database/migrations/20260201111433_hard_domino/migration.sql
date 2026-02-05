ALTER TABLE "images" ADD COLUMN "disk_image_key" text;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "pending_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "starting_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "running_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "stopping_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "stopped_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "vms" ADD COLUMN "failed_at" timestamp with time zone;