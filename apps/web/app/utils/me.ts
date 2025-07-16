export function useMe() {
  return {
    token: process.env.ME_TOKEN,
  }
}
