CREATE TABLE "waitlist" (
	"id" uuid PRIMARY KEY NOT NULL,
	"email" text NOT NULL,
	"x_forwarded_for" text,
	"country" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "waitlist_email_unique" UNIQUE("email")
);
