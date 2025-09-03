export default defineNuxtPlugin(() => {
  const router = useRouter()

  let navigationStack: string[] = []

  router.beforeEach((to, from) => {
    if (process.client) {
      const isBackNavigation = window.performance.navigation?.type === 2 ||
                               (window.history.state && window.history.state.back === true)

      if (isBackNavigation || navigationStack.includes(to.path)) {
        // Going back
        if (to.meta.layoutTransition && typeof to.meta.layoutTransition !== 'boolean') {
          to.meta.layoutTransition.name = 'layout-back'
        }
        // Remove paths after current one from stack
        const index = navigationStack.indexOf(to.path)
        if (index !== -1) {
          navigationStack = navigationStack.slice(0, index + 1)
        }
      } else {
        // Going forward
        if (to.meta.layoutTransition && typeof to.meta.layoutTransition !== 'boolean') {
          to.meta.layoutTransition.name = 'layout-forward'
        }
        navigationStack.push(from.path)
      }
    }
  })
})
