/**
 * Provider & Model Catalog Service.
 */
import {
  fetchModelsForProvider,
  listProviders,
  type ProviderCatalogEntry,
} from "@/agent/src/commands/provider-registry.js";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import type { Model, ProviderId } from "@console/types";

export class ProviderService {
  constructor(private storage: SqliteSessionStorage = new SqliteSessionStorage()) {}

  getProviders(): ProviderCatalogEntry[] {
    const providers = listProviders();
    try {
      const favorites = this.storage.listModelFavorites();
      if (favorites.length === 0) return providers;

      const favSet = new Set(favorites.map((f) => `${f.provider}:${f.modelId}`));
      return providers.map((provider) => ({
        ...provider,
        models: [...provider.models].sort((a, b) => {
          const aFav = favSet.has(`${provider.name}:${a.id}`);
          const bFav = favSet.has(`${provider.name}:${b.id}`);
          if (aFav && !bFav) return -1;
          if (!aFav && bFav) return 1;
          return 0;
        }),
      }));
    } catch {
      return providers;
    }
  }

  async getModels(providerId: ProviderId): Promise<Model[]> {
    const models = await fetchModelsForProvider(providerId);
    try {
      const favorites = this.storage.listModelFavorites();
      const favModelIds = new Set(
        favorites.filter((f) => f.provider === providerId).map((f) => f.modelId),
      );
      if (favModelIds.size === 0) return models;

      return [...models].sort((a, b) => {
        const aFav = favModelIds.has(a.id);
        const bFav = favModelIds.has(b.id);
        if (aFav && !bFav) return -1;
        if (!aFav && bFav) return 1;
        return 0;
      });
    } catch {
      return models;
    }
  }
}
