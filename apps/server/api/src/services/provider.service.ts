/**
 * Provider & Model Catalog Service.
 */
import {
  fetchModelsForProvider,
  listProviders,
  type ProviderCatalogEntry,
} from "../../../agent/src/commands/provider-registry.js";
import type { Model, ProviderId } from "@console/types";

export class ProviderService {
  getProviders(): ProviderCatalogEntry[] {
    return listProviders();
  }

  async getModels(providerId: ProviderId): Promise<Model[]> {
    return fetchModelsForProvider(providerId);
  }
}
