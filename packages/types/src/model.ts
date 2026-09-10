/**
 * Shared model/provider types.
 * `ProviderId` is the single source of truth for valid provider names.
 */

export type ProviderId = "antigravity" | "opencode" | "codex" | "cline" | "devin";

export type ThinkingLevel = "none" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";

/** Providers that authenticate via OAuth (login-only providers). */
export type OAuthProviderId = "antigravity" | "codex" | "devin";

export interface Model {
  id: string;
  provider: ProviderId;
  contextWindow: number;
  /** Whether the provider explicitly reports support for image input. */
  supportsImages?: boolean;
  /** Thinking levels accepted by this model. */
  supportedThinkingLevels?: ThinkingLevel[];
  /** Default thinking level when no runtime override is supplied. */
  defaultThinkingLevel?: ThinkingLevel;
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
  authMethod: "oauth" | "device-code" | "none" | "api-key";
}
