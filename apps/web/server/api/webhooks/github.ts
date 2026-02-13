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

      if (organisation) {
        const { error: insertError } = await tryCatch(
          useDrizzle().insert(schema.githubInstallations).values({
            githubAccountId: githubAccountId,
            githubInstallationId: installationId,
            organisationId: organisation.id,
            userId: "123",
          }),
        );
        if (insertError) throw new Error("Error inserting installation");
      }
      break;
    case "deleted":
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

  // Find the organization by installation ID
  const { data: installationRecords, error: installationError } = await tryCatch(
    useDrizzle()
      .select({
        organisation: schema.organisations,
        installation: schema.githubInstallations,
      })
      .from(schema.githubInstallations)
      .innerJoin(
        schema.organisations,
        eq(schema.organisations.id, schema.githubInstallations.organisationId),
      )
      .where(eq(schema.githubInstallations.githubInstallationId, installationId))
      .limit(1),
  );
  const installationRecord = installationRecords?.[0] ?? null;
  if (!installationRecord || installationError) {
    throw new Error("Installation not found");
  }

  const organisation = installationRecord.organisation;

  // Fetch repository info (log errors but don't fail)
  const { error: repoError } = await useGitHub().repository.get(
    installationId,
    githubOwner,
    githubRepo,
  );
  if (repoError) {
    throw new Error("Failed to fetch repo");
  }

  // Fetch commit info (log errors but don't fail)
  const { error: commitError } = await useGitHub().commit.get(
    installationId,
    githubOwner,
    githubRepo,
    commitSHA,
  );
  if (commitError) {
    throw new Error("Failed to fetch commit info");
  }

  // Fetch the project using githubRepository field
  const githubRepository = `${githubOwner}/${githubRepo}`;
  const { data: projectsList, error: findProjectError } = await tryCatch<any[]>(
    useDrizzle()
      .select()
      .from(schema.projects)
      .where(eq(schema.projects.githubRepository, githubRepository))
      .limit(1),
  );
  const project = projectsList?.[0];
  if (findProjectError || !project) {
    throw new Error("Project not found");
  }

  // Create deployment
  const deploymentModel = useDeploymentModel();
  const { error: deploymentError } = await deploymentModel.create({
    projectId: project.id,
    organisationId: organisation.id,
  });

  if (deploymentError) {
    throw new Error("Failed to create deployment");
  }
}
