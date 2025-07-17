// auth.d.ts
declare module "#auth-utils" {
  interface User {
    id: number // GitHub ID
    name: string
    email: string
    username: string
    githubId: number
    avatarUrl: string
    accessToken: string
    userId?: string // Database user ID (UUID)
  }

  interface UserSession {
    // Add your own fields
  }

  interface SecureSessionData {
    // Add your own fields
  }
}

export {}
