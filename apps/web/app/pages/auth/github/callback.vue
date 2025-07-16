<!-- <script setup lang="ts">
import { cacheExchange, Client, fetchExchange } from "@urql/core"
import { provideClient, useMutation } from "@urql/vue"
import { graphql } from "~~/app/gql"

definePageMeta({
  layout: "auth",
})

async function login(code: string) {
  try {
    const { data, error } = await loginWithGitHub({ code })
    if (error) {
      throw error
    }

    console.log(data)
  } catch (error) {
    console.error(error)
  }
}

onMounted(async () => {
  nextTick(async () => {
    const code = useRoute().query.code as string
    if (!code) {
      throw new Error("No code provided")
    }

    await login(code)
  })
})

// mutations
const { executeMutation: loginWithGitHub } = useMutation(
  graphql(/* GraphQL */ `
    mutation loginWithGitHub($code: String!) {
      loginWithGitHub(code: $code) {
        token
        user {
          id
          username
        }
      }
    }
  `),
)
</script> -->

<template>
  <div>
    <div>loading...</div>
  </div>
</template>
