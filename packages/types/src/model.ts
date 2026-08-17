/**
 * Shared model/provider types.
 * `ProviderId` is the single source of truth for valid provider names.
 */

export type ProviderId = "gemini" | "antigravity" | "opencode" | "codebuff";

/** Providers that authenticate via Google OAuth (login-only providers). */
export type OAuthProviderId = "gemini" | "antigravity";

export interface Model {
  id: string;
  provider: ProviderId;
  contextWindow: number;
  /** Whether the provider explicitly reports support for image input. */
  supportsImages?: boolean;
}

export interface ModelFavorite {
  provider: ProviderId;
  modelId: string;
}

export interface ProviderCatalogEntry {
  name: ProviderId;
  displayName: string;
  description: string;
  models: Model[];
  /** How the user authenticates with this provider (drives the Account UI). */
  authMethod: "oauth" | "device-code" | "none";
}
