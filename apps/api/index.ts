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
import { eq, and, isNull, or } from "drizzle-orm";
import { addMinutes, isAfter, isBefore } from "date-fns";
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

  // pending deployments - only process one per organization to avoid overload
  const allPendingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "pending"));

  if (allPendingDeployments.length > 0) {
    console.log(
      `[RECONCILE] Found ${allPendingDeployments.length} pending deployment(s)`
    );
  }

  // Get all active deployments (building or deploying) to check which orgs are busy
  const activeDeploymentsForOrgs = await useDrizzle()
    .select()
    .from(deployments)
    .where(
      or(
        eq(deployments.status, "building"),
        eq(deployments.status, "deploying")
      )
    );

  // Create a set of organization IDs that already have active deployments
  const busyOrgIds = new Set(
    activeDeploymentsForOrgs.map((d) => d.organisationId)
  );

  if (busyOrgIds.size > 0) {
    console.log(
      `[RECONCILE] ${busyOrgIds.size} organization(s) already have active deployments`
    );
  }

  // Group by organisation_id and take only the first deployment from each organization
  // that doesn't already have an active deployment
  const orgDeploymentMap = new Map<string, (typeof allPendingDeployments)[0]>();
  for (const deployment of allPendingDeployments) {
    // Skip if this organization already has an active deployment
    if (busyOrgIds.has(deployment.organisationId)) {
      continue;
    }

    // Take only the first pending deployment per organization
    if (!orgDeploymentMap.has(deployment.organisationId)) {
      orgDeploymentMap.set(deployment.organisationId, deployment);
    }
  }

  const pendingDeployments = Array.from(orgDeploymentMap.values());

  if (pendingDeployments.length > 0) {
    console.log(
      `[RECONCILE] Processing ${pendingDeployments.length} deployment(s) (1 per organization, skipping busy orgs)`
    );
  }

  for (const deployment of pendingDeployments) {
    console.log(`[DEPLOYMENT:${deployment.id}] Processing pending deployment`, {
      projectId: deployment.projectId,
      environmentId: deployment.environmentId,
      githubCommit: deployment.githubCommit,
    });

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
          console.error(
            `[DEPLOYMENT:${deployment.id}] Failed to find project or environment`,
            {
              projectFound: !!project,
              environmentFound: !!environment,
            }
          );
          throw new Error("Project or environment not found");
        }

        console.log(`[DEPLOYMENT:${deployment.id}] Creating image build`, {
          repository: project.githubRepository,
          commit: deployment.githubCommit,
          githubInstallationId: project.githubInstallationId,
        });

        const [imageBuild] = await tx
          .insert(imageBuilds)
          .values({
            githubRepository: project.githubRepository,
            githubCommit: deployment.githubCommit,
            githubInstallationId: project.githubInstallationId,
          })
          .returning();

        if (!imageBuild) {
          console.error(
            `[DEPLOYMENT:${deployment.id}] Failed to create image build record`
          );
          throw new Error("Failed to create image build");
        }

        console.log(
          `[DEPLOYMENT:${deployment.id}] Transitioning pending → building`,
          {
            imageBuildId: imageBuild.id,
          }
        );

        await tx
          .update(deployments)
          .set({
            status: "building",
            imageBuildId: imageBuild.id,
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));

        console.log(
          `[DEPLOYMENT:${deployment.id}] State changed: pending → building`
        );
      });
    } catch (error) {
      console.error(
        `[DEPLOYMENT:${deployment.id}] Failed to process pending deployment:`,
        error
      );
    }
  }

  // building deployments
  const buildingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "building"));

  if (buildingDeployments.length > 0) {
    console.log(
      `[RECONCILE] Found ${buildingDeployments.length} building deployment(s)`
    );
  }

  for (const deployment of buildingDeployments) {
    // if the build is completed then mark the deployment as deploying and create deployment instance with a instance and mark it as deploying
    // if the build has failed then mark the deployment as failed
    try {
      await useDrizzle().transaction(async (tx) => {
        if (!deployment.imageBuildId) {
          console.error(`[DEPLOYMENT:${deployment.id}] Missing image build ID`);
          throw new Error("Image build not found");
        }

        const [imageBuild] = await tx
          .select()
          .from(imageBuilds)
          .where(eq(imageBuilds.id, deployment.imageBuildId));

        if (!imageBuild) {
          console.error(
            `[DEPLOYMENT:${deployment.id}] Image build not found in database`,
            {
              imageBuildId: deployment.imageBuildId,
            }
          );
          throw new Error("Image build not found");
        }

        console.log(`[DEPLOYMENT:${deployment.id}] Checking build status`, {
          buildStatus: imageBuild.status,
          imageBuildId: imageBuild.id,
        });

        if (imageBuild.status === "completed") {
          const region = regionList[0];
          if (!region) {
            console.error(`[DEPLOYMENT:${deployment.id}] No regions available`);
            throw new Error("No region found");
          }

          const node = nodeList.find((n) => n.regionId === region.id);

          if (!node) {
            console.error(
              `[DEPLOYMENT:${deployment.id}] No region or node available`,
              {
                regionId: region?.id,
                nodesInRegion: nodeList.filter((n) => n.regionId === region?.id)
                  .length,
              }
            );
            throw new Error("Region or node not found");
          }

          if (!deployment.imageId) {
            if (!imageBuild.imageId) {
              console.error(
                `[DEPLOYMENT:${deployment.id}] Build completed but no image ID`,
                {
                  imageBuildId: imageBuild.id,
                }
              );
              throw new Error("Image not found");
            }

            console.log(
              `[DEPLOYMENT:${deployment.id}] Linking image to deployment`,
              {
                imageId: imageBuild.imageId,
              }
            );

            // check if the build has a image id and if so update the deployment with the image id
            await tx
              .update(deployments)
              .set({
                imageId: imageBuild.imageId,
                updatedAt: new Date(),
              })
              .where(eq(deployments.id, deployment.id));

            return;
          }

          console.log(`[DEPLOYMENT:${deployment.id}] Creating instance`, {
            regionId: region.id,
            nodeId: node.id,
            imageId: deployment.imageId,
          });

          // Allocate an IP address for the instance
          const allocatedIP = await allocateIPAddress(region.id);

          console.log(
            `[DEPLOYMENT:${deployment.id}] Allocated IP for instance`,
            {
              ipAddress: allocatedIP,
            }
          );

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
              ipAddress: allocatedIP,
              imageId: deployment.imageId,
            })
            .returning();

          if (!instance) {
            console.error(
              `[DEPLOYMENT:${deployment.id}] Failed to create instance record`
            );
            throw new Error("Failed to create instance");
          }

          console.log(`[DEPLOYMENT:${deployment.id}] Instance created`, {
            instanceId: instance.id,
          });

          await tx.insert(deploymentInstances).values({
            deploymentId: deployment.id,
            instanceId: instance.id,
            organisationId: deployment.organisationId,
          });

          console.log(
            `[DEPLOYMENT:${deployment.id}] Transitioning building → deploying`
          );

          await tx
            .update(deployments)
            .set({
              status: "deploying",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));

          console.log(
            `[DEPLOYMENT:${deployment.id}] State changed: building → deploying`
          );
        } else if (imageBuild.status === "failed") {
          console.log(
            `[DEPLOYMENT:${deployment.id}] Build failed, marking deployment as failed`,
            {
              imageBuildId: imageBuild.id,
            }
          );

          await tx
            .update(deployments)
            .set({
              status: "failed",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));

          // mark all instances associated with this deployment as stopping
          const deploymentInstancesList = await tx
            .select()
            .from(deploymentInstances)
            .where(eq(deploymentInstances.deploymentId, deployment.id));

          for (const di of deploymentInstancesList) {
            await tx
              .update(instances)
              .set({
                state: "stopping",
                updatedAt: new Date(),
              })
              .where(eq(instances.id, di.instanceId));
          }

          if (deploymentInstancesList.length > 0) {
            console.log(
              `[DEPLOYMENT:${deployment.id}] Marked ${deploymentInstancesList.length} instance(s) as stopping`
            );
          }

          console.log(
            `[DEPLOYMENT:${deployment.id}] State changed: building → failed (build failed)`
          );
        } else if (imageBuild.status === "pending") {
          // Still waiting for build to start
          return;
        } else if (imageBuild.status === "building") {
          // Still building, wait for completion
          return;
        } else {
          console.error(
            `[DEPLOYMENT:${deployment.id}] Invalid build status: ${imageBuild.status}`
          );
          throw new Error("Image build status is not valid");
        }
      });
    } catch (error) {
      console.error(
        `[DEPLOYMENT:${deployment.id}] Failed to process building deployment:`,
        error
      );
    }
  }

  // deploying deployments
  const deployingDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "deploying"));

  if (deployingDeployments.length > 0) {
    console.log(
      `[RECONCILE] Found ${deployingDeployments.length} deploying deployment(s)`
    );
  }

  for (const deployment of deployingDeployments) {
    // Check if any instances are running, if so mark deployment as active
    try {
      const deploymentInstancesList = await useDrizzle()
        .select()
        .from(deploymentInstances)
        .where(eq(deploymentInstances.deploymentId, deployment.id));

      // If no deployment instances exist, create them (this is a new deploying deployment)
      if (deploymentInstancesList.length === 0) {
        console.log(
          `[DEPLOYMENT:${deployment.id}] Deploying deployment has no instances, creating them`
        );

        await useDrizzle().transaction(async (tx) => {
          // Verify deployment has an image
          if (!deployment.imageId) {
            console.error(
              `[DEPLOYMENT:${deployment.id}] Cannot create instances - no image ID`
            );
            throw new Error("Deployment has no image");
          }

          const region = regionList[0];
          if (!region) {
            console.error(`[DEPLOYMENT:${deployment.id}] No regions available`);
            throw new Error("No region found");
          }

          const node = nodeList.find((n) => n.regionId === region.id);

          if (!node) {
            console.error(
              `[DEPLOYMENT:${deployment.id}] No region or node available`,
              {
                regionId: region?.id,
                nodesInRegion: nodeList.filter((n) => n.regionId === region?.id)
                  .length,
              }
            );
            throw new Error("Region or node not found");
          }

          console.log(`[DEPLOYMENT:${deployment.id}] Creating instance`, {
            regionId: region.id,
            nodeId: node.id,
            imageId: deployment.imageId,
          });

          // Allocate an IP address for the instance
          const allocatedIP = await allocateIPAddress(region.id);

          console.log(
            `[DEPLOYMENT:${deployment.id}] Allocated IP for instance`,
            {
              ipAddress: allocatedIP,
            }
          );

          // Create instance
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
              ipAddress: allocatedIP,
              imageId: deployment.imageId,
            })
            .returning();

          if (!instance) {
            console.error(
              `[DEPLOYMENT:${deployment.id}] Failed to create instance record`
            );
            throw new Error("Failed to create instance");
          }

          console.log(`[DEPLOYMENT:${deployment.id}] Instance created`, {
            instanceId: instance.id,
          });

          // Create deployment instance link
          await tx.insert(deploymentInstances).values({
            deploymentId: deployment.id,
            instanceId: instance.id,
            organisationId: deployment.organisationId,
          });

          console.log(
            `[DEPLOYMENT:${deployment.id}] Deployment instance link created`
          );

          // Update deployment timestamp to reset timeout counter
          await tx
            .update(deployments)
            .set({
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));
        });

        console.log(
          `[DEPLOYMENT:${deployment.id}] Instances created, waiting for them to start`
        );
        continue;
      }

      // Check if any instance is in running state
      let hasRunningInstance = false;
      for (const di of deploymentInstancesList) {
        const instance = await useDrizzle().query.instances.findFirst({
          where: eq(instances.id, di.instanceId),
        });

        if (instance && instance.state === "running") {
          hasRunningInstance = true;
          break;
        }
      }

      if (hasRunningInstance) {
        await useDrizzle()
          .update(deployments)
          .set({
            status: "active",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));

        console.log(
          `[DEPLOYMENT:${deployment.id}] State changed: deploying → active (instance running)`
        );
        continue;
      }

      // Check if any instance has failed status
      let hasFailedInstance = false;
      for (const di of deploymentInstancesList) {
        const instance = await useDrizzle().query.instances.findFirst({
          where: eq(instances.id, di.instanceId),
        });

        if (instance && instance.state === "failed") {
          hasFailedInstance = true;
          break;
        }
      }

      if (hasFailedInstance) {
        console.log(
          `[DEPLOYMENT:${deployment.id}] Instance failed, marking deployment as failed`
        );

        await useDrizzle().transaction(async (tx) => {
          await tx
            .update(deployments)
            .set({
              status: "failed",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, deployment.id));

          // mark all instances associated with this deployment as stopping
          const deploymentInstancesList = await tx
            .select()
            .from(deploymentInstances)
            .where(eq(deploymentInstances.deploymentId, deployment.id));

          for (const di of deploymentInstancesList) {
            await tx
              .update(instances)
              .set({
                state: "stopping",
                updatedAt: new Date(),
              })
              .where(eq(instances.id, di.instanceId));
          }

          if (deploymentInstancesList.length > 0) {
            console.log(
              `[DEPLOYMENT:${deployment.id}] Marked ${deploymentInstancesList.length} instance(s) as stopping`
            );
          }

          console.log(
            `[DEPLOYMENT:${deployment.id}] State changed: deploying → failed (instance failed)`
          );
        });

        continue;
      }

      // if the deployment is deploying and hasn't been updated in the last 5 minutes then mark the deployment as failed
      const lastUpdate = deployment.updatedAt;
      const timeoutThreshold = addMinutes(new Date(), -5);

      // skip if the deployment was updated within the last 5 minutes
      if (isAfter(lastUpdate, timeoutThreshold)) {
        console.log(`[DEPLOYMENT:${deployment.id}] Deploying timeout check`, {
          lastUpdate,
          timeoutThreshold,
          timedOut: false,
        });
        continue;
      }

      console.log(
        `[DEPLOYMENT:${deployment.id}] Deployment timed out (no update in 5+ minutes)`,
        {
          lastUpdate,
          minutesElapsed: Math.round(
            (new Date().getTime() - lastUpdate.getTime()) / 60000
          ),
        }
      );

      await useDrizzle().transaction(async (tx) => {
        await tx
          .update(deployments)
          .set({
            status: "failed",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));

        // mark all instances associated with this deployment as stopping
        const deploymentInstancesList = await tx
          .select()
          .from(deploymentInstances)
          .where(eq(deploymentInstances.deploymentId, deployment.id));

        for (const di of deploymentInstancesList) {
          await tx
            .update(instances)
            .set({
              state: "stopping",
              updatedAt: new Date(),
            })
            .where(eq(instances.id, di.instanceId));
        }

        if (deploymentInstancesList.length > 0) {
          console.log(
            `[DEPLOYMENT:${deployment.id}] Marked ${deploymentInstancesList.length} instance(s) as stopping`
          );
        }

        console.log(
          `[DEPLOYMENT:${deployment.id}] State changed: deploying → failed (timeout)`
        );
      });
    } catch (error) {
      console.error(
        `[DEPLOYMENT:${deployment.id}] Failed to process deploying deployment:`,
        error
      );
    }
  }

  // active deployments
  const activeDeployments = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "active"));

  if (activeDeployments.length > 0) {
    console.log(
      `[RECONCILE] Found ${activeDeployments.length} active deployment(s)`
    );
  }

  // Check if active deployments actually have deployment instances and instances
  // If not, reschedule them
  for (const deployment of activeDeployments) {
    try {
      const deploymentInstancesList = await useDrizzle()
        .select()
        .from(deploymentInstances)
        .where(eq(deploymentInstances.deploymentId, deployment.id));

      if (deploymentInstancesList.length === 0) {
        console.log(
          `[DEPLOYMENT:${deployment.id}] Active deployment has no deployment instances, rescheduling`,
          {
            status: "active → deploying",
          }
        );

        await useDrizzle()
          .update(deployments)
          .set({
            status: "deploying",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));

        console.log(
          `[DEPLOYMENT:${deployment.id}] State changed: active → deploying (missing deployment instances)`
        );
        continue;
      }

      // Check if the deployment instances have valid instances
      let hasValidInstance = false;
      for (const di of deploymentInstancesList) {
        const instance = await useDrizzle().query.instances.findFirst({
          where: eq(instances.id, di.instanceId),
        });

        if (instance) {
          hasValidInstance = true;
          break;
        }
      }

      if (!hasValidInstance) {
        console.log(
          `[DEPLOYMENT:${deployment.id}] Active deployment has no valid instances, rescheduling`,
          {
            status: "active → deploying",
            deploymentInstancesCount: deploymentInstancesList.length,
          }
        );

        await useDrizzle()
          .update(deployments)
          .set({
            status: "deploying",
            updatedAt: new Date(),
          })
          .where(eq(deployments.id, deployment.id));

        console.log(
          `[DEPLOYMENT:${deployment.id}] State changed: active → deploying (missing instances)`
        );
      }
    } catch (error) {
      console.error(
        `[DEPLOYMENT:${deployment.id}] Failed to check deployment instances:`,
        error
      );
    }
  }

  // Refresh active deployments list after potential rescheduling
  const activeDeploymentsRefreshed = await useDrizzle()
    .select()
    .from(deployments)
    .where(eq(deployments.status, "active"));

  // Group active deployments by project/environment
  const deploymentGroups = new Map<string, typeof activeDeploymentsRefreshed>();
  for (const deployment of activeDeploymentsRefreshed) {
    const key = `${deployment.projectId}-${deployment.environmentId}`;
    if (!deploymentGroups.has(key)) {
      deploymentGroups.set(key, []);
    }
    deploymentGroups.get(key)!.push(deployment);
  }

  // Process each group
  for (const [groupKey, groupDeployments] of deploymentGroups) {
    // Sort by ID descending (newest first, since UUIDv7 is time-ordered)
    const sortedDeployments = groupDeployments.sort((a, b) => {
      if (a.id > b.id) return -1;
      if (a.id < b.id) return 1;
      return 0;
    });

    console.log(`[RECONCILE] Processing deployment group ${groupKey}`, {
      totalActive: sortedDeployments.length,
    });

    // Keep the latest deployment (index 0) active
    const latest = sortedDeployments[0];
    if (!latest) continue;

    console.log(`[DEPLOYMENT:${latest.id}] Latest deployment, keeping active`);

    // Process the second deployment (N-1)
    if (sortedDeployments.length > 1) {
      const prior = sortedDeployments[1];
      if (!prior) continue;

      const fiveMinutesAgo = addMinutes(new Date(), -5);
      const latestBecameActiveAt = latest.updatedAt;

      if (isBefore(latestBecameActiveAt, fiveMinutesAgo)) {
        // Latest has been active for more than 5 minutes, stop the prior deployment
        console.log(
          `[DEPLOYMENT:${prior.id}] Prior deployment grace period expired, marking as inactive`,
          {
            latestDeploymentId: latest.id,
            minutesSinceLatestActive: Math.round(
              (new Date().getTime() - latestBecameActiveAt.getTime()) / 60000
            ),
          }
        );

        await useDrizzle().transaction(async (tx) => {
          await tx
            .update(deployments)
            .set({
              status: "inactive",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, prior.id));

          // Mark instances as stopping
          const deploymentInstancesList = await tx
            .select()
            .from(deploymentInstances)
            .where(eq(deploymentInstances.deploymentId, prior.id));

          for (const di of deploymentInstancesList) {
            await tx
              .update(instances)
              .set({
                state: "stopping",
                updatedAt: new Date(),
              })
              .where(eq(instances.id, di.instanceId));
          }

          if (deploymentInstancesList.length > 0) {
            console.log(
              `[DEPLOYMENT:${prior.id}] Marked ${deploymentInstancesList.length} instance(s) as stopping`
            );
          }

          console.log(
            `[DEPLOYMENT:${prior.id}] State changed: active → inactive (grace period expired)`
          );
        });
      } else {
        console.log(
          `[DEPLOYMENT:${prior.id}] Prior deployment in grace period, keeping active`,
          {
            latestDeploymentId: latest.id,
            minutesSinceLatestActive: Math.round(
              (new Date().getTime() - latestBecameActiveAt.getTime()) / 60000
            ),
          }
        );
      }
    }

    // Process all older deployments (N-2, N-3, etc.) - stop them immediately
    if (sortedDeployments.length > 2) {
      const olderDeployments = sortedDeployments.slice(2);
      console.log(
        `[RECONCILE] Found ${olderDeployments.length} old deployment(s) to stop immediately`
      );

      for (const oldDeployment of olderDeployments) {
        console.log(
          `[DEPLOYMENT:${oldDeployment.id}] Old deployment, marking as inactive immediately`,
          {
            latestDeploymentId: latest.id,
          }
        );

        await useDrizzle().transaction(async (tx) => {
          await tx
            .update(deployments)
            .set({
              status: "inactive",
              updatedAt: new Date(),
            })
            .where(eq(deployments.id, oldDeployment.id));

          // Mark instances as stopping
          const deploymentInstancesList = await tx
            .select()
            .from(deploymentInstances)
            .where(eq(deploymentInstances.deploymentId, oldDeployment.id));

          for (const di of deploymentInstancesList) {
            await tx
              .update(instances)
              .set({
                state: "stopping",
                updatedAt: new Date(),
              })
              .where(eq(instances.id, di.instanceId));
          }

          if (deploymentInstancesList.length > 0) {
            console.log(
              `[DEPLOYMENT:${oldDeployment.id}] Marked ${deploymentInstancesList.length} instance(s) as stopping`
            );
          }

          console.log(
            `[DEPLOYMENT:${oldDeployment.id}] State changed: active → inactive (superseded)`
          );
        });
      }
    }
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

  if (failedDeployments.length > 0) {
    console.log(
      `[RECONCILE] Found ${failedDeployments.length} failed deployment(s)`
    );
  }

  for (const deployment of failedDeployments) {
    try {
      // mark all instances associated with this deployment as stopping
      const deploymentInstancesList = await useDrizzle()
        .select()
        .from(deploymentInstances)
        .where(eq(deploymentInstances.deploymentId, deployment.id));

      for (const di of deploymentInstancesList) {
        const instance = await useDrizzle().query.instances.findFirst({
          where: eq(instances.id, di.instanceId),
        });

        // only mark as stopping if not already stopping, stopped, or terminated
        if (
          instance &&
          instance.state !== "stopping" &&
          instance.state !== "stopped" &&
          instance.state !== "terminated"
        ) {
          await useDrizzle()
            .update(instances)
            .set({
              state: "stopping",
              updatedAt: new Date(),
            })
            .where(eq(instances.id, di.instanceId));

          console.log(
            `[DEPLOYMENT:${deployment.id}] Marked instance ${instance.id} as stopping (was ${instance.state})`
          );
        }
      }
    } catch (error) {
      console.error(
        `[DEPLOYMENT:${deployment.id}] Failed to process failed deployment:`,
        error
      );
    }
  }

  // IMAGE BUILDS

  // if an image build has status building and the updated_at is older than 10 minutes then mark it as failed (timed out)
  const buildingImageBuilds = await useDrizzle()
    .select()
    .from(imageBuilds)
    .where(eq(imageBuilds.status, "building"));

  if (buildingImageBuilds.length > 0) {
    console.log(
      `[RECONCILE] Found ${buildingImageBuilds.length} building image build(s)`
    );
  }

  for (const imageBuild of buildingImageBuilds) {
    const lastUpdate = imageBuild.updatedAt;
    const timeout = addMinutes(new Date(), -10);

    if (isBefore(lastUpdate, timeout)) {
      console.log(
        `[IMAGE_BUILD:${imageBuild.id}] Build timed out (no update in 10+ minutes)`,
        {
          repository: imageBuild.githubRepository,
          commit: imageBuild.githubCommit,
          lastUpdate,
          minutesElapsed: Math.round(
            (new Date().getTime() - lastUpdate.getTime()) / 60000
          ),
        }
      );

      await useDrizzle().transaction(async (tx) => {
        await tx
          .update(imageBuilds)
          .set({
            status: "failed",
            updatedAt: new Date(),
          })
          .where(eq(imageBuilds.id, imageBuild.id));

        console.log(
          `[IMAGE_BUILD:${imageBuild.id}] State changed: building → failed (timeout)`
        );
      });
    }
  }

  // DOMAINS

  // if there is a new domain and it has verified_at null and internal is false then try to verify it (check if the verification token is set for the txt record of the domain)
  const newDomains = await useDrizzle()
    .select()
    .from(domains)
    .where(and(isNull(domains.verifiedAt), eq(domains.internal, false)));

  if (newDomains.length > 0) {
    console.log(`[RECONCILE] Found ${newDomains.length} unverified domain(s)`);
  }

  for (const domain of newDomains) {
    // has the domain a verification token
    if (!domain.verificationToken) {
      console.log(
        `[DOMAIN:${domain.id}] Skipping verification - no token set`,
        {
          domainName: domain.name,
        }
      );
      continue;
    }

    console.log(`[DOMAIN:${domain.id}] Attempting DNS verification`, {
      domainName: domain.name,
      txtRecord: `_zeitwork-verify-token.${domain.name}`,
    });

    // try to verify it (check if the verification token is set for the txt record of the domain)
    try {
      const results = await dns.promises.resolveTxt(
        `_zeitwork-verify-token.${domain.name}`
      );

      console.log(`[DOMAIN:${domain.id}] DNS TXT records found`, {
        domainName: domain.name,
        recordCount: results.length,
      });

      // check if any of the results contains the verification token
      const verificationToken = results.find((result) =>
        result.includes(domain.verificationToken!)
      );

      if (verificationToken) {
        console.log(`[DOMAIN:${domain.id}] Verification successful`, {
          domainName: domain.name,
        });

        await useDrizzle().transaction(async (tx) => {
          await tx
            .update(domains)
            .set({ verifiedAt: new Date(), updatedAt: new Date() })
            .where(eq(domains.id, domain.id));

          console.log(
            `[DOMAIN:${domain.id}] Domain verified and marked as verified`
          );
        });
      } else {
        console.log(
          `[DOMAIN:${domain.id}] Verification token not found in DNS records`,
          {
            domainName: domain.name,
          }
        );
      }
    } catch (error) {
      console.log(`[DOMAIN:${domain.id}] DNS lookup failed`, {
        domainName: domain.name,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }
}

/*** HELPER FUNCTIONS ***/

/**
 * Allocates a single /32 IP address for a Firecracker VM
 * Uses 10.77.0.0/16 range organized into /29 subnets for networking
 *
 * Architecture:
 * - Each VM index maps to a /29 subnet with tap device
 * - Tap device uses .1 address, VM gets single .2 address (/32)
 * - Example: VM index 0 gets 10.77.0.2/32, tap at 10.77.0.1/29
 * - Example: VM index 1 gets 10.77.0.10/32, tap at 10.77.0.9/29
 *
 * This allows up to 8192 VMs (indices 0-8191)
 */
async function allocateIPAddress(regionId: string): Promise<string> {
  // Get all existing IP addresses from instances (all regions share same internal IP space)
  const existingInstances = await useDrizzle()
    .select({ ipAddress: instances.ipAddress })
    .from(instances);

  const usedIPs = new Set<string>();

  // Add all used IPs to the set
  existingInstances.forEach((inst) => usedIPs.add(inst.ipAddress));

  // Allocate single /32 IP addresses from 10.77.0.0/16 range
  // VMs are organized into /29 subnets for tap device networking
  // Each /29 has: .0 (network), .1 (tap), .2 (VM /32), .3-.6 (unused), .7 (broadcast)
  for (let vmIndex = 0; vmIndex < 8192; vmIndex++) {
    const offset = vmIndex * 8; // Each VM gets 8 IPs worth of space
    const octet3 = Math.floor(offset / 256);
    const octet4 = (offset % 256) + 2; // .2 is the VM's single /32 address
    const candidateIP = `10.77.${octet3}.${octet4}`;

    if (!usedIPs.has(candidateIP)) {
      console.log(
        `[IP_ALLOCATE] Allocated /32 IP: ${candidateIP} (VM index ${vmIndex})`
      );
      return candidateIP;
    }
  }

  throw new Error(
    `No available IP addresses in 10.77.0.0/16 range (all 8192 VM slots used)`
  );
}

console.log("[API] Starting reconciliation loop");

while (true) {
  try {
    console.log("[RECONCILE] Starting reconciliation cycle");
    const startTime = Date.now();
    await reconcile();
    const duration = Date.now() - startTime;
    console.log(`[RECONCILE] Reconciliation cycle completed in ${duration}ms`);
  } catch (error) {
    console.error("[RECONCILE] Reconciliation cycle failed:", error);
  }
  await new Promise((resolve) => setTimeout(resolve, 1000));
}
