## deployment

if a deployment does not have an image build, create an image build and update the deployment status to building

if a deployment is building and the image build is completed update the deployment status to deploying

if a deployment has status deploying then create deployment_instances and instances for the deployment

if a deployment has healthy instances then update the deployment status to active

if a deployment is marked as active, check if it is the latest deployment in the project environment and if so then update the deploymentId for the domains associcated with the deployments project environment (none internal domains)
