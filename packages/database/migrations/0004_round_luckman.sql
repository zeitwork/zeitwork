CREATE TYPE "public"."ssl_certificate_statuses" AS ENUM('pending', 'active', 'failed', 'renewing');--> statement-breakpoint
ALTER TABLE "domains" ADD COLUMN "ssl_certificate_status" "ssl_certificate_statuses" DEFAULT 'pending';--> statement-breakpoint
ALTER TABLE "domains" ADD COLUMN "ssl_certificate_issued_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "domains" ADD COLUMN "ssl_certificate_expires_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "domains" ADD COLUMN "ssl_certificate_error" text;