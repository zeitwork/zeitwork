import { useDeploymentModel } from "~~/server/models/deployment";
import { tryCatch } from "~~/server/utils/tryCatch";
import * as schema from "@zeitwork/database/schema";
import { Webhooks } from "@octokit/webhooks";

export default defineEventHandler(async (event) => {
  const eventType = getHeader(event, "x-github-event");

  const { data: rawBody, error: bodyError } = await tryCatch(readRawBody(event));
  if (bodyError || !rawBody) {
    throw createError({
      statusCode: 400,
      statusMessage: "Invalid body",
    });
  }

  const { error: signatureError } = await tryCatch(verifySignature(event, rawBody));
  if (signatureError) {
    throw createError({
      statusCode: 400,
      statusMessage: "Invalid signature",
    });
  }

  const { data: payload, error: parseError } = await tryCatch(JSON.parse(rawBody));
  if (parseError) {
    throw createError({
      statusCode: 400,
      statusMessage: "Parsing body failed",
    });
  }

  switch (eventType) {
    case "installation":
      const { error: installationError } = await tryCatch(handleInstallationEvent(payload));
      if (installationError) {
        throw createError({
          statusCode: 500,
          statusMessage: installationError.message,
        });
      }
      return "ok";
    case "push":
      const { error: eventError } = await tryCatch(handlePushEvent(payload));
      if (eventError) {
        throw createError({
          statusCode: 500,
          statusMessage: eventError.message,
        });
      }
      return "ok";
    case "installation_repositories":
      break;
  }

  return "ok";
});

async function verifySignature(event: any, rawBody: string) {
  const signature = getHeader(event, "x-hub-signature-256");
  const webhookSecret = useRuntimeConfig().githubWebhookSecret;
  if (!signature) {
    throw createError({
      statusCode: 401,
      statusMessage: "Missing signature",
    });
  }
  if (!webhookSecret) {
    throw createError({
      statusCode: 500,
      statusMessage: "Webhook secret not configured",
    });
  }

  const webhooks = new Webhooks({ secret: webhookSecret });

  const { data: isValid, error: verifyError } = await tryCatch(webhooks.verify(rawBody, signature));
  if (verifyError) {
    throw createError({
      statusCode: 401,
      statusMessage: "Signature verification failed",
    });
  }
  if (!isValid) {
    throw createError({
      statusCode: 401,
      statusMessage: "Invalid signature",
    });
  }
}

async function handleInstallationEvent(payload: any) {
  const installationId = payload.installation?.id;
  if (!installationId) throw new Error("Missing installationId");

  switch (payload.action) {
    case "created":
      const githubLogin = payload.installation?.account?.login?.toLowerCase();
      const githubAccountId = payload.installation?.account?.id;

      if (!githubLogin || !githubAccountId || !installationId) {
        throw new Error("Missing required fields");
      }

      // Look up the organization by slug
      const { data: organisationList, error: organizationListError } = await tryCatch(
        useDrizzle()
          .select()
          .from(schema.organisations)
          .where(eq(schema.organisations.slug, githubLogin))
          .limit(1),
      );
      if (organizationListError) throw new Error("Organization not found");

      const organisation = organisationList[0];
      if (!organisation) throw new Error("Organization not found");

      // Look up the user by their GitHub account ID
      const { data: userList, error: userLookupError } = await tryCatch(
        useDrizzle()
          .select()
          .from(schema.users)
          .where(eq(schema.users.githubAccountId, githubAccountId))
          .limit(1),
      );
      if (userLookupError) throw new Error("Error looking up user");

      const user = userList?.[0];
      if (!user) {
        // User doesn't have an account yet — the OAuth flow will create the
        // github_installations record on their first login via the
        // pending_installation cookie path.
        return;
      }

      const { error: insertError } = await tryCatch(
        useDrizzle().insert(schema.githubInstallations).values({
          githubAccountId: githubAccountId,
          githubInstallationId: installationId,
          organisationId: organisation.id,
          userId: user.id,
        }),
      );
      if (insertError) throw new Error("Error inserting installation");
      break;
    case "deleted":
      // TODO: in the future, consider also soft-deleting projects (and their
      // deployments) that reference this installation via githubInstallationId.
      // For now we only remove the installation record itself; orphaned projects
      // will simply stop receiving new deployments (push handler returns early
      // on "Installation not found").
      const { error: deleteError } = await tryCatch(
        useDrizzle()
          .delete(schema.githubInstallations)
          .where(eq(schema.githubInstallations.githubInstallationId, installationId)),
      );
      if (deleteError) {
        throw new Error("Error deleting installation");
      }
      break;
    default:
      return new Error("Not implemented");
  }
}

async function handlePushEvent(payload: any) {
  const ref = payload.ref;
  if (ref !== "refs/heads/main") {
    return;
  }

  const installationId = payload.installation?.id;
  const githubOwner = payload.repository?.owner?.login;
  const githubRepo = payload.repository?.name;
  const commitSHA = payload.after;

  if (!installationId || !githubOwner || !githubRepo || !commitSHA) {
    throw new Error("Missing required fields");
  }

  // A push that deletes a branch sends the all-zeros SHA — ignore it.
  const DELETED_BRANCH_SHA = "0000000000000000000000000000000000000000";
  if (commitSHA === DELETED_BRANCH_SHA) {
    return;
  }

  // Find the installation record to scope project lookups
  const { data: installationRecords, error: installationError } = await tryCatch(
    useDrizzle()
      .select()
      .from(schema.githubInstallations)
      .where(eq(schema.githubInstallations.githubInstallationId, installationId))
      .limit(1),
  );
  const installationRecord = installationRecords?.[0] ?? null;
  if (!installationRecord || installationError) {
    throw new Error("Installation not found");
  }

  // Fetch all projects using this githubRepository scoped to the triggering installation
  const githubRepository = `${githubOwner}/${githubRepo}`;
  const { data: projectsList, error: findProjectError } = await tryCatch<any[]>(
    useDrizzle()
      .select()
      .from(schema.projects)
      .where(
        and(
          eq(schema.projects.githubRepository, githubRepository),
          eq(schema.projects.githubInstallationId, installationRecord.id),
          isNull(schema.projects.deletedAt),
        ),
      ),
  );
  if (findProjectError) {
    throw new Error("Failed to query projects");
  }
  if (!projectsList || projectsList.length === 0) {
    // No projects configured for this repo yet — not an error.
    return;
  }

  // Create a deployment for each project linked to this repository
  const deploymentModel = useDeploymentModel();
  for (const project of projectsList) {
    const { error: deploymentError } = await deploymentModel.create({
      projectId: project.id,
      organisationId: project.organisationId,
    });

    if (deploymentError) {
      throw new Error(`Failed to create deployment for project ${project.slug}`);
    }
  }
}
