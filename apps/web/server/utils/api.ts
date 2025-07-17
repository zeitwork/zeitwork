type ZeitworkResponse<T> =
  | {
      data: null
      error: Error
    }
  | {
      data: T
      error: null
    }

function useZeitworkClient() {
  async function createOrganisation(options: any): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface GetOrganisationOptions {
    organisationId: string
  }

  async function getOrganisation(options: GetOrganisationOptions): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  async function listOrganisations(): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  async function createProject(options: any): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface ListProjectsOptions {
    organisationId: string
  }

  async function listProjects(options: ListProjectsOptions): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface GetProjectOptions {
    organisationId: string
    projectId: string
  }

  async function getProject(options: GetProjectOptions): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  async function installGitHubForOrganisation(options: any): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface GetDeploymentOptions {
    projectId: string
    deploymentId: string
    organisationId: string
  }

  async function getDeployment(options: GetDeploymentOptions): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  interface ListDeploymentsOptions {
    projectId: string
    organisationId: string
  }

  async function listDeployments(options: ListDeploymentsOptions): Promise<ZeitworkResponse<null>> {
    return { data: null, error: new Error("Not implemented") }
  }

  return {
    projects: {
      get: getProject,
      list: listProjects,
      create: createProject,
    },
    deployments: {
      get: getDeployment,
      list: listDeployments,
    },
    organisations: {
      get: getOrganisation,
      list: listOrganisations,
      create: createOrganisation,
    },
  }
}
