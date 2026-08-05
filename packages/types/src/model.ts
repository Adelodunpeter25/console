export interface Model {
  id: string;
  provider: "gemini" | "antigravity";
  contextWindow: number;
  /** Whether the provider explicitly reports support for image input. */
  supportsImages?: boolean;
}

export interface ProviderCatalogEntry {
  name: "gemini" | "antigravity";
  displayName: string;
  description: string;
  models: Model[];
}
