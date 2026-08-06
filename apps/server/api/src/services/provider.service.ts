/**
 * Provider & Model Catalog Service.
 */
import {
  fetchModelsForProvider,
  listProviders,
  type ProviderCatalogEntry,
} from "../../../agent/src/commands/provider-registry.js";
import type { Model } from "../../../agent/src/types/index.js";

export class ProviderService {
  getProviders(): ProviderCatalogEntry[] {
    return listProviders();
  }

  async getModels(providerId: "gemini" | "antigravity" | "opencode"): Promise<Model[]> {
    return fetchModelsForProvider(providerId);
  }
}
