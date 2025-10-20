ALTER TABLE "builds" ADD COLUMN "github_commit" text NOT NULL;--> statement-breakpoint
ALTER TABLE "builds" ADD COLUMN "github_branch" text NOT NULL;