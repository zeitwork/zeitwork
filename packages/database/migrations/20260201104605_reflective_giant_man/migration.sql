CREATE TYPE "build_status" AS ENUM('pending', 'building', 'succesful', 'failed');--> statement-breakpoint
CREATE TYPE "deployment_status" AS ENUM('pending', 'building', 'starting', 'running', 'stopping', 'stopped', 'failed');--> statement-breakpoint
CREATE TYPE "vm_status" AS ENUM('pending', 'starting', 'running', 'stopping', 'stopped', 'failed');--> statement-breakpoint
CREATE TABLE "build_logs" (
	"id" uuid PRIMARY KEY,
	"build_id" uuid NOT NULL,
	"message" text NOT NULL,
	"level" text NOT NULL,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "builds" (
	"id" uuid PRIMARY KEY,
	"status" "build_status" DEFAULT 'pending'::"build_status" NOT NULL,
	"project_id" uuid NOT NULL,
	"github_commit" text NOT NULL,
	"github_branch" text NOT NULL,
	"image_id" uuid,
	"vm_id" uuid,
	"pending_at" timestamp with time zone,
	"building_at" timestamp with time zone,
	"successful_at" timestamp with time zone,
	"failed_at" timestamp with time zone,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "certmagic_data" (
	"key" text PRIMARY KEY,
	"value" text NOT NULL,
	"modified" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "certmagic_locks" (
	"key" text PRIMARY KEY,
	"expires" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "deployment_logs" (
	"id" uuid PRIMARY KEY,
	"deployment_id" uuid NOT NULL,
	"message" text NOT NULL,
	"level" text,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "deployments" (
	"id" uuid PRIMARY KEY,
	"status" "deployment_status" DEFAULT 'pending'::"deployment_status" NOT NULL,
	"github_commit" text NOT NULL,
	"project_id" uuid NOT NULL,
	"build_id" uuid,
	"image_id" uuid,
	"vm_id" uuid,
	"pending_at" timestamp with time zone,
	"building_at" timestamp with time zone,
	"starting_at" timestamp with time zone,
	"running_at" timestamp with time zone,
	"stopping_at" timestamp with time zone,
	"stopped_at" timestamp with time zone,
	"failed_at" timestamp with time zone,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "domains" (
	"id" uuid PRIMARY KEY,
	"name" text NOT NULL,
	"project_id" uuid NOT NULL,
	"deployment_id" uuid,
	"verified_at" timestamp with time zone,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone,
	CONSTRAINT "domains_name_project_id_unique" UNIQUE("name","project_id")
);
--> statement-breakpoint
CREATE TABLE "environment_variables" (
	"id" uuid PRIMARY KEY,
	"name" text NOT NULL,
	"value" text NOT NULL,
	"project_id" uuid NOT NULL,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone,
	CONSTRAINT "environment_variables_name_project_id_unique" UNIQUE("name","project_id")
);
--> statement-breakpoint
CREATE TABLE "github_installations" (
	"id" uuid PRIMARY KEY,
	"user_id" uuid NOT NULL,
	"github_account_id" integer NOT NULL,
	"github_installation_id" integer NOT NULL UNIQUE,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "images" (
	"id" uuid PRIMARY KEY,
	"registry" text NOT NULL,
	"repository" text NOT NULL,
	"tag" text NOT NULL,
	"digest" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "organisation_members" (
	"id" uuid PRIMARY KEY,
	"user_id" uuid NOT NULL,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "organisations" (
	"id" uuid PRIMARY KEY,
	"name" text NOT NULL,
	"slug" text NOT NULL UNIQUE,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "projects" (
	"id" uuid PRIMARY KEY,
	"name" text NOT NULL,
	"slug" text NOT NULL,
	"github_repository" text NOT NULL,
	"github_installation_id" uuid NOT NULL,
	"organisation_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone,
	CONSTRAINT "projects_slug_organisation_id_unique" UNIQUE("slug","organisation_id")
);
--> statement-breakpoint
CREATE TABLE "users" (
	"id" uuid PRIMARY KEY,
	"name" text NOT NULL,
	"email" text NOT NULL UNIQUE,
	"username" text NOT NULL UNIQUE,
	"profile_picture_url" text,
	"github_account_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "vms" (
	"id" uuid PRIMARY KEY,
	"vcpus" integer NOT NULL,
	"memory" integer NOT NULL,
	"status" "vm_status" NOT NULL,
	"image_id" uuid NOT NULL,
	"port" integer,
	"ip_address" inet NOT NULL,
	"metadata" jsonb,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "build_logs" ADD CONSTRAINT "build_logs_build_id_builds_id_fkey" FOREIGN KEY ("build_id") REFERENCES "builds"("id");--> statement-breakpoint
ALTER TABLE "build_logs" ADD CONSTRAINT "build_logs_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "builds" ADD CONSTRAINT "builds_project_id_projects_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects"("id");--> statement-breakpoint
ALTER TABLE "builds" ADD CONSTRAINT "builds_image_id_images_id_fkey" FOREIGN KEY ("image_id") REFERENCES "images"("id");--> statement-breakpoint
ALTER TABLE "builds" ADD CONSTRAINT "builds_vm_id_vms_id_fkey" FOREIGN KEY ("vm_id") REFERENCES "vms"("id");--> statement-breakpoint
ALTER TABLE "builds" ADD CONSTRAINT "builds_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "deployment_logs" ADD CONSTRAINT "deployment_logs_deployment_id_deployments_id_fkey" FOREIGN KEY ("deployment_id") REFERENCES "deployments"("id");--> statement-breakpoint
ALTER TABLE "deployment_logs" ADD CONSTRAINT "deployment_logs_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_project_id_projects_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects"("id");--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_build_id_builds_id_fkey" FOREIGN KEY ("build_id") REFERENCES "builds"("id");--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_image_id_images_id_fkey" FOREIGN KEY ("image_id") REFERENCES "images"("id");--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_vm_id_vms_id_fkey" FOREIGN KEY ("vm_id") REFERENCES "vms"("id");--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "domains" ADD CONSTRAINT "domains_project_id_projects_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects"("id");--> statement-breakpoint
ALTER TABLE "domains" ADD CONSTRAINT "domains_deployment_id_deployments_id_fkey" FOREIGN KEY ("deployment_id") REFERENCES "deployments"("id");--> statement-breakpoint
ALTER TABLE "domains" ADD CONSTRAINT "domains_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "environment_variables" ADD CONSTRAINT "environment_variables_project_id_projects_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects"("id");--> statement-breakpoint
ALTER TABLE "environment_variables" ADD CONSTRAINT "environment_variables_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "github_installations" ADD CONSTRAINT "github_installations_user_id_users_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id");--> statement-breakpoint
ALTER TABLE "github_installations" ADD CONSTRAINT "github_installations_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "organisation_members" ADD CONSTRAINT "organisation_members_user_id_users_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id");--> statement-breakpoint
ALTER TABLE "organisation_members" ADD CONSTRAINT "organisation_members_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_github_installation_id_github_installations_id_fkey" FOREIGN KEY ("github_installation_id") REFERENCES "github_installations"("id");--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_organisation_id_organisations_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisations"("id");--> statement-breakpoint
ALTER TABLE "vms" ADD CONSTRAINT "vms_image_id_images_id_fkey" FOREIGN KEY ("image_id") REFERENCES "images"("id");
