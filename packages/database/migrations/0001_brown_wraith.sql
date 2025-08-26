CREATE TABLE "build_queue" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" uuid,
	"image_id" uuid,
	"priority" integer DEFAULT 0 NOT NULL,
	"status" text NOT NULL,
	"github_repo" text NOT NULL,
	"commit_hash" text NOT NULL,
	"branch" text NOT NULL,
	"build_started_at" timestamp with time zone,
	"build_completed_at" timestamp with time zone,
	"build_log" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "ipv6_allocations" (
	"id" uuid PRIMARY KEY NOT NULL,
	"region_id" uuid,
	"node_id" uuid,
	"instance_id" uuid,
	"ipv6_address" text NOT NULL,
	"prefix" text NOT NULL,
	"state" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone,
	CONSTRAINT "ipv6_allocations_ipv6Address_unique" UNIQUE("ipv6_address")
);
--> statement-breakpoint
CREATE TABLE "routing_cache" (
	"id" uuid PRIMARY KEY NOT NULL,
	"domain" text NOT NULL,
	"deployment_id" uuid,
	"instances" jsonb NOT NULL,
	"version" integer DEFAULT 1 NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone,
	CONSTRAINT "routing_cache_domain_unique" UNIQUE("domain")
);
--> statement-breakpoint
CREATE TABLE "tls_certificates" (
	"id" uuid PRIMARY KEY NOT NULL,
	"domain" text NOT NULL,
	"certificate" text NOT NULL,
	"private_key" text NOT NULL,
	"issuer" text NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"auto_renew" boolean DEFAULT true NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "images" ADD COLUMN "s3_bucket" text;--> statement-breakpoint
ALTER TABLE "images" ADD COLUMN "s3_key" text;--> statement-breakpoint
ALTER TABLE "images" ADD COLUMN "builder_node_id" uuid;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "deployment_url" text;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "nanoid" text;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "rollout_strategy" text DEFAULT 'blue-green' NOT NULL;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "min_instances" integer DEFAULT 3 NOT NULL;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "activated_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "deployments" ADD COLUMN "deactivated_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "build_queue" ADD CONSTRAINT "build_queue_image_id_images_id_fk" FOREIGN KEY ("image_id") REFERENCES "public"."images"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ipv6_allocations" ADD CONSTRAINT "ipv6_allocations_region_id_regions_id_fk" FOREIGN KEY ("region_id") REFERENCES "public"."regions"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ipv6_allocations" ADD CONSTRAINT "ipv6_allocations_node_id_nodes_id_fk" FOREIGN KEY ("node_id") REFERENCES "public"."nodes"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ipv6_allocations" ADD CONSTRAINT "ipv6_allocations_instance_id_instances_id_fk" FOREIGN KEY ("instance_id") REFERENCES "public"."instances"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "images" ADD CONSTRAINT "images_builder_node_id_nodes_id_fk" FOREIGN KEY ("builder_node_id") REFERENCES "public"."nodes"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_nanoid_unique" UNIQUE("nanoid");