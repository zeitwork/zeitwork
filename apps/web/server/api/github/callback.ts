export default defineEventHandler(async (event) => {
  const code = getQuery(event).code
  const installationId = getQuery(event).installation_id
  const setupAction = getQuery(event).setup_action
  const state = getQuery(event).state

  console.log(code, installationId, setupAction, state)

  return {}
})
