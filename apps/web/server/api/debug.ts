export default defineEventHandler(async (event) => {
  const { user } = await getUserSession(event)

  setResponseHeader(event, "Content-Type", "application/json")

  const config = useRuntimeConfig(event)

  console.log("--------------------------------")
  console.log("")
  console.log("")
  console.log("")
  console.log(config.kubeConfig)
  console.log("")
  console.log("")
  console.log("")
  console.log("--------------------------------")

  return {
    user,
  }
})
