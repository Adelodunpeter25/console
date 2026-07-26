/**
 * Desktop-specific auth response types.
 *
 * These mirror the `data` payload returned by the Console server's auth
 * routes — distinct from the DTOs and `AuthStatusResponse` already shared
 * via `@console/types`.
 */

export interface LoginUrlResult {
  provider: string;
  authUrl: string;
  redirectUri: string;
}

export interface OAuthCallbackResult {
  provider: string;
  userEmail?: string;
  projectId?: string;
}
