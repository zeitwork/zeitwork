ALTER TABLE "organisations" ADD COLUMN "stripe_customer_id" text;--> statement-breakpoint
ALTER TABLE "organisations" ADD COLUMN "stripe_subscription_id" text;--> statement-breakpoint
ALTER TABLE "organisations" ADD COLUMN "stripe_subscription_status" text;