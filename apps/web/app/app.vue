<script setup lang="ts">
import { cacheExchange, Client, fetchExchange } from "@urql/core"
import { provideClient } from "@urql/vue"

const { user } = useUserSession()

const client = new Client({
  url: useRuntimeConfig().public.graphEndpoint,
  exchanges: [cacheExchange, fetchExchange],
  fetchOptions: () => {
    const me = useMe()

    return {
      headers: {
        Authorization: user.value?.accessToken ?? "",
      },
    }
  },
})

provideClient(client)
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
