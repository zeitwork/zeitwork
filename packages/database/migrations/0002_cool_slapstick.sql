CREATE TABLE "logs" (
	"id" uuid PRIMARY KEY NOT NULL,
	"image_build_id" uuid,
	"instance_id" uuid,
	"level" text,
	"message" text NOT NULL,
	"logged_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "logs" ADD CONSTRAINT "logs_image_build_id_image_builds_id_fk" FOREIGN KEY ("image_build_id") REFERENCES "public"."image_builds"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "logs" ADD CONSTRAINT "logs_instance_id_instances_id_fk" FOREIGN KEY ("instance_id") REFERENCES "public"."instances"("id") ON DELETE no action ON UPDATE no action;