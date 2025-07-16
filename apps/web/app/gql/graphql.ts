/* eslint-disable */
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  Time: { input: any; output: any; }
};

export type CreateProjectInput = {
  githubOwner: Scalars['String']['input'];
  githubRepo: Scalars['String']['input'];
  name: Scalars['String']['input'];
};

export type CreateProjectPayload = {
  __typename?: 'CreateProjectPayload';
  project: Project;
};

export type Deployment = {
  __typename?: 'Deployment';
  id: Scalars['ID']['output'];
  organisation: Organisation;
  previewUrl: Scalars['String']['output'];
  project: Project;
};

export type DeploymentConnection = {
  __typename?: 'DeploymentConnection';
  nodes: Array<Deployment>;
};

export type Domain = {
  __typename?: 'Domain';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  organisation: Organisation;
};

export type LoginWithGitHubPayload = {
  __typename?: 'LoginWithGitHubPayload';
  user: User;
};

export type MePayload = {
  __typename?: 'MePayload';
  user: User;
};

export type Mutation = {
  __typename?: 'Mutation';
  createProject: CreateProjectPayload;
  loginWithGitHub: LoginWithGitHubPayload;
  setInstallationID: SetInstallationIdPayload;
};


export type MutationCreateProjectArgs = {
  input: CreateProjectInput;
};


export type MutationSetInstallationIdArgs = {
  installationId: Scalars['String']['input'];
  organisationId: Scalars['ID']['input'];
};

export type Organisation = {
  __typename?: 'Organisation';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type OrganisationConnection = {
  __typename?: 'OrganisationConnection';
  nodes: Array<Organisation>;
};

export type Project = {
  __typename?: 'Project';
  deployments: DeploymentConnection;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  organisation: Organisation;
  slug: Scalars['String']['output'];
};

export type ProjectConnection = {
  __typename?: 'ProjectConnection';
  nodes: Array<Project>;
};

export type ProjectsInput = {
  organisationId: Scalars['ID']['input'];
};

export type Query = {
  __typename?: 'Query';
  me: MePayload;
  projects: ProjectConnection;
};


export type QueryProjectsArgs = {
  input: ProjectsInput;
};

export type SetInstallationIdPayload = {
  __typename?: 'SetInstallationIDPayload';
  organisation: Organisation;
};

export type User = {
  __typename?: 'User';
  githubId: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  organisations: OrganisationConnection;
  username: Scalars['String']['output'];
};

export type Project_ProjectFragmentFragment = { __typename?: 'Project', id: string, name: string, slug: string } & { ' $fragmentName'?: 'Project_ProjectFragmentFragment' };

export type ProjectsQueryVariables = Exact<{
  orgId: Scalars['ID']['input'];
}>;


export type ProjectsQuery = { __typename?: 'Query', projects: { __typename?: 'ProjectConnection', nodes: Array<(
      { __typename?: 'Project' }
      & { ' $fragmentRefs'?: { 'Project_ProjectFragmentFragment': Project_ProjectFragmentFragment } }
    )> } };

export type MeQueryVariables = Exact<{ [key: string]: never; }>;


export type MeQuery = { __typename?: 'Query', me: { __typename?: 'MePayload', user: { __typename?: 'User', id: string, username: string, githubId: number, organisations: { __typename?: 'OrganisationConnection', nodes: Array<{ __typename?: 'Organisation', id: string, name: string, slug: string }> } } } };

export type CreateProjectMutationVariables = Exact<{
  input: CreateProjectInput;
}>;


export type CreateProjectMutation = { __typename?: 'Mutation', createProject: { __typename?: 'CreateProjectPayload', project: { __typename?: 'Project', id: string, name: string, slug: string, organisation: { __typename?: 'Organisation', id: string, name: string, slug: string } } } };

export const Project_ProjectFragmentFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"Project_ProjectFragment"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}}]}}]} as unknown as DocumentNode<Project_ProjectFragmentFragment, unknown>;
export const ProjectsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Projects"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orgId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projects"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"ObjectValue","fields":[{"kind":"ObjectField","name":{"kind":"Name","value":"organisationId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orgId"}}}]}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"Project_ProjectFragment"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"Project_ProjectFragment"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}}]}}]} as unknown as DocumentNode<ProjectsQuery, ProjectsQueryVariables>;
export const MeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"username"}},{"kind":"Field","name":{"kind":"Name","value":"githubId"}},{"kind":"Field","name":{"kind":"Name","value":"organisations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<MeQuery, MeQueryVariables>;
export const CreateProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateProjectInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"project"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"organisation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}}]}}]}}]}}]}}]} as unknown as DocumentNode<CreateProjectMutation, CreateProjectMutationVariables>;