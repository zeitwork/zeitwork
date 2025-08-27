ALTER TABLE "projects" ADD COLUMN "github_repo" text;--> statement-breakpoint
ALTER TABLE "projects" ADD COLUMN "github_installation_id" integer;--> statement-breakpoint
ALTER TABLE "projects" ADD COLUMN "github_default_branch" text DEFAULT 'main';--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_github_installation_id_github_installations_id_fk" FOREIGN KEY ("github_installation_id") REFERENCES "public"."github_installations"("id") ON DELETE no action ON UPDATE no action;