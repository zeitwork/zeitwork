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
  token: Scalars['String']['output'];
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


export type MutationLoginWithGitHubArgs = {
  code: Scalars['String']['input'];
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

export type LoginWithGitHubMutationVariables = Exact<{
  code: Scalars['String']['input'];
}>;


export type LoginWithGitHubMutation = { __typename?: 'Mutation', loginWithGitHub: { __typename?: 'LoginWithGitHubPayload', token: string, user: { __typename?: 'User', id: string, username: string } } };


export const LoginWithGitHubDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"loginWithGitHub"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loginWithGitHub"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"token"}},{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"username"}}]}}]}}]}}]} as unknown as DocumentNode<LoginWithGitHubMutation, LoginWithGitHubMutationVariables>;