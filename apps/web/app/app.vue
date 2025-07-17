<script setup lang="ts">
import { cacheExchange, Client, fetchExchange } from "@urql/core"
import { provideClient } from "@urql/vue"

const url = computed(() => useRuntimeConfig().apiUrl + "/graph")

const client = new Client({
  // url: url.value,
  url: "https://api.zeitwork.com/graph",
  exchanges: [cacheExchange, fetchExchange],
  fetchOptions: () => {
    const { user } = useUserSession()

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
