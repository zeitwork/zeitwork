import {
  deploymentInstances,
  deployments,
  domains,
  imageBuilds,
  instances,
  nodes,
  projectEnvironments,
  projects,
  regions,
} from "../../packages/database/schema";
import { useDrizzle } from "./drizzle";
import { eq, gt, and, isNull } from "drizzle-orm";
import { addMinutes, isBefore } from "date-fns";
import * as dns from "node:dns";

async function reconcile() {
  const regionList = await useDrizzle().select().from(regions);
  if (regionList.length === 0) {
    throw new Error("No regions found");
  }

  const nodeList = await useDrizzle().select().from(nodes);
  if (nodeList.length === 0) {
    throw new Error("No nodes found");
  }

  // pending deployments
  const pendingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "pending"));
  for (const deployment of pendingDeployments) {
    try {
      // create a build for the deployment
      await useDrizzle().transaction(async (tx) => {
        const project = await tx.query.projects.findFirst({
          where: eq(projects.id, deployment.projectId),
        });
        const environment = await tx.query.projectEnvironments.findFirst({
          where: eq(projectEnvironments.id, deployment.environmentId),
        });

        if (!project || !environment) {
          throw new Error("Project or environment not found");
        }

        const [imageBuild] = await tx
          .insert(imageBuilds)
          .values({
            githubRepository: project.githubRepository,
            githubCommit: deployment.githubCommit,
          })
          .returning();

        if (!imageBuild) {
          throw new Error("Failed to create image build");
        }

        await tx
          .update(deployments)
          .set({
            status: "building",
            imageBuildId: imageBuild.id,
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));
      });
    } catch (error) {
      console.error(error);
    }
  }

  // building deployments
  const buildingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "building"));
  for (const deployment of buildingDeployments) {
    // if the build is completed then mark the deployment as deploying and create deployment instance with a instance and mark it as deploying
    // if the build has failed then mark the deployment as failed
    try {
      await useDrizzle().transaction(async (tx) => {
        if (!deployment.imageBuildId) {
          throw new Error("Image build not found");
        }

        const [imageBuild] = await tx
          .select()
          .from(imageBuilds)
          .where(eq(imageBuilds.id, deployment.imageBuildId));

        if (!imageBuild) {
          throw new Error("Image build not found");
        }

        if (imageBuild.status === "completed") {
          const region = regionList[0];
          const node = nodeList[0];

          if (!region || !node) {
            throw new Error("Region or node not found");
          }

          if (!deployment.imageId) {
            throw new Error("Image not found");
          }

          // create a deployment instance with a instance and mark it as deploying
          const [instance] = await tx
            .insert(instances)
            .values({
              state: "pending",
              regionId: region.id,
              nodeId: node.id,
              defaultPort: 3000,
              vcpus: 2,
              memory: 2048,
              environmentVariables: JSON.stringify({}),
              ipAddress: "127.0.0.1",
              imageId: deployment.imageId,
            })
            .returning();

          if (!instance) {
            throw new Error("Failed to create instance");
          }

          await tx.insert(deploymentInstances).values({
            deploymentId: deployment.id,
            instanceId: instance.id,
            organisationId: deployment.organisationId,
          });

          await tx
            .update(deployments)
            .set({
              status: "deploying",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));
        } else if (imageBuild.status === "failed") {
          await tx
            .update(deployments)
            .set({
              status: "failed",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));
        } else {
          throw new Error("Image build status is not valid");
        }
      });
    } catch (error) {
      console.error(error);
    }
  }

  // deploying deployments
  const deployingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "deploying"));
  for (const deployment of deployingDeployments) {
    // if the deployment is deploying and hasn't been updated in the last 5 minutes then mark the deployment as failed
    try {
      // skip if the deployment updated at is less than 5 minutes
      if (isBefore(deployment.updatedAt, addMinutes(new Date(), -5))) {
        continue;
      }

      await useDrizzle().transaction(async (tx) => {
        await tx
          .update(deployments)
          .set({
            status: "failed",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));
      });
    } catch (error) {
      console.error(error);
    }
  }

  // active deployments
  const activeDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "active"));
  for (const deployment of activeDeployments) {
    // check if there is a newer deployment for the project/environment
    // if the current deployment is older than 5 minutes then mark it as inactive
    // btw deployment.id are uuidv7

    await useDrizzle().transaction(async (tx) => {
      // simply check if there is a deployment greater than the current deployment id
      const [newerDeployment] = await tx
        .select()
        .from(deployments)
        .where(gt(deployments.id, deployment.id));

      if (newerDeployment) {
        await tx
          .update(deployments)
          .set({
            status: "inactive",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));
      } else {
        await tx
          .update(deployments)
          .set({
            status: "active",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));
      }
    });
  }

  // inactive deployments
  const inactiveDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "inactive"));
  for (const deployment of inactiveDeployments) {
    // do nothing
  }

  // failed deployments
  const failedDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "failed"));
  for (const deployment of failedDeployments) {
    // do nothing
  }

  // IMAGE BUILDS

  // if an image build has status building and the updated_at is older than 10 minutes then mark it as failed (timed out)
  const buildingImageBuilds = await useDrizzle()
    .select()
    .from(imageBuilds)
    .where(eq(imageBuilds.status, "building"));
  for (const imageBuild of buildingImageBuilds) {
    if (isBefore(imageBuild.updatedAt, addMinutes(new Date(), -10))) {
      await useDrizzle().transaction(async (tx) => {
        await tx
          .update(imageBuilds)
          .set({
            status: "failed",
            updatedAt: new Date(),
          })
          .where(eq(imageBuilds.id, imageBuild.id));
      });
    }
  }

  // DOMAINS

  // if there is a new domain and it has verified_at null and internal is false then try to verify it (check if the verification token is set for the txt record of the domain)
  const newDomains = await useDrizzle()
    .select()
    .from(domains)
    .where(and(isNull(domains.verifiedAt), eq(domains.internal, false)));
  for (const domain of newDomains) {
    // has the domain a verification token
    if (!domain.verificationToken) {
      continue;
    }

    // try to verify it (check if the verification token is set for the txt record of the domain)
    const results = await dns.promises.resolveTxt(
      `_zeitwork-verify-token.${domain.name}`
    );

    // check if any of the results contains the verification token
    const verificationToken = results.find((result) =>
      result.includes(domain.verificationToken!)
    );

    if (verificationToken) {
      await useDrizzle().transaction(async (tx) => {
        await tx
          .update(domains)
          .set({ verifiedAt: new Date(), updatedAt: new Date() })
          .where(eq(domains.id, domain.id));
      });
    }
  }
}

while (true) {
  try {
    console.log("Reconciling...");
    await reconcile();
  } catch (error) {
    console.error(error);
  }
  await new Promise((resolve) => setTimeout(resolve, 1000));
}
