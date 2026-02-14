ALTER TABLE "images" DROP CONSTRAINT "images_building_by_servers_id_fkey";--> statement-breakpoint
ALTER TABLE "images" DROP COLUMN "disk_image_key";--> statement-breakpoint
ALTER TABLE "images" DROP COLUMN "building_by";--> statement-breakpoint
ALTER TABLE "images" DROP COLUMN "building_started_at";