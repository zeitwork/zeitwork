ALTER TABLE "vms" ALTER COLUMN "server_no" DROP NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "server_name" DROP NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "server_type" DROP NOT NULL;--> statement-breakpoint
ALTER TABLE "vms" ALTER COLUMN "public_ip" DROP NOT NULL;