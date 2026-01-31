// auth.d.ts
declare module "#auth-utils" {
  interface User {
    id: string; // Database user ID (UUID)
    name: string;
    email: string;
    username: string;
    githubId: number;
    avatarUrl: string;
  }

  interface UserSession {
    hasSubscription?: boolean;
    subscriptionCheckedAt?: number; // timestamp
  }

  interface SecureSessionData {
    userId: string;
    organisationId: string;
    tokens: {
      access_token: string;
      expires_in: number;
      refresh_token: string;
      refresh_token_expires_in: number;
      token_type: string;
      scope: string;
    };
  }
}

export {};
