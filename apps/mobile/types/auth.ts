/** Result of GET /api/auth/login/url. */
export interface LoginUrlResult {
  provider: string;
  authUrl: string;
  redirectUri: string;
  state: string;
}
