/**
 * OAuth credential types.
 * Covers both the raw file format (multiple naming conventions across Gemini CLI versions)
 * and the normalised, validated form used throughout the providers.
 */

/**
 * Raw shape of the credential file at ~/.gemini/oauth_creds.json.
 * The Gemini CLI has used different field names across versions —
 * we support all of them and normalise in parseCredential().
 */
export interface GeminiOAuthCredential {
  /** Access token — current Gemini CLI (v0.35+) */
  access_token?: string;
  /** Access token — older Gemini CLI versions */
  token?: string;
  /** Expiry as epoch ms — current Gemini CLI */
  expiry_date?: number;
  /** Expiry — alternative field names */
  expiresAt?: number;
  expires?: number;
  /** Refresh token */
  refresh_token?: string;
  /** Older field name for refresh token */
  refreshToken?: string;
  refresh?: string;
  /** Project ID — newer Gemini CLI */
  projectId?: string;
  /** Project ID — older Gemini CLI */
  project_id?: string;
  /** OAuth scopes */
  scope?: string;
  /** Token type (typically "Bearer") */
  token_type?: string;
  /** ID token (JWT) */
  id_token?: string;
  /** Account email */
  email?: string;
}

/**
 * Normalised credential — guaranteed non-empty accessToken and projectId.
 * All optional fields use explicit `string | undefined` or `number | undefined`
 * so callers never need to guess.
 */
export interface ParsedCredential {
  accessToken: string;
  projectId: string;
  refreshToken: string | undefined;
  /** Epoch ms */
  expiresAtMs: number | undefined;
  email: string | undefined;
}
