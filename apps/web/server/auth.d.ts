// auth.d.ts
declare module "#auth-utils" {
  interface User {
    // Add your own fields
    accessToken: string
  }

  interface UserSession {
    // Add your own fields
  }

  interface SecureSessionData {
    // Add your own fields
  }
}

export {}
