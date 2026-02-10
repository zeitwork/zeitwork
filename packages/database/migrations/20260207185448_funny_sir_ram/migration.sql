ALTER TABLE "deployment_logs" RENAME TO "vm_logs";--> statement-breakpoint
ALTER TABLE "vm_logs" DROP CONSTRAINT "deployment_logs_deployment_id_deployments_id_fkey";--> statement-breakpoint
ALTER TABLE "vm_logs" DROP CONSTRAINT "deployment_logs_organisation_id_organisations_id_fkey";--> statement-breakpoint
ALTER TABLE "vm_logs" ADD COLUMN "vm_id" uuid NOT NULL;--> statement-breakpoint
ALTER TABLE "vm_logs" DROP COLUMN "deployment_id";--> statement-breakpoint
ALTER TABLE "vm_logs" DROP COLUMN "organisation_id";--> statement-breakpoint
ALTER TABLE "deployments" ADD CONSTRAINT "deployments_vm_id_key" UNIQUE("vm_id");--> statement-breakpoint
ALTER TABLE "vm_logs" ADD CONSTRAINT "vm_logs_vm_id_vms_id_fkey" FOREIGN KEY ("vm_id") REFERENCES "vms"("id");