// auth.d.ts
declare module "#auth-utils" {
  interface User {
    id: string // Database user ID (UUID)
    name: string
    email: string
    username: string
    githubId: number
    avatarUrl: string
  }

  interface UserSession {
    // Add your own fields
  }

  interface SecureSessionData {
    userId: string
  }
}

export {}
