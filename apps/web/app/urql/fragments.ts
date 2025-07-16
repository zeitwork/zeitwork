import { graphql } from "~/gql"

export const Project_ProjectFragment = graphql(/* GraphQL */ `
  fragment Project_ProjectFragment on Project {
    id
    name
    slug
  }
`)
