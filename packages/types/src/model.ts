export interface Model {
  id: string;
  provider: "gemini" | "antigravity";
  contextWindow: number;
}

export interface ProviderCatalogEntry {
  name: "gemini" | "antigravity";
  displayName: string;
  description: string;
  models: Model[];
}
