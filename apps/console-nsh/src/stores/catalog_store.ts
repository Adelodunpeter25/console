// The catalog store: the provider/model catalog, the user's favorite
// models, and the picked model — the data behind the launcher's model
// picker (the desktop's `providers.rs` state). Loaded once and cached; a
// favorite toggle updates the local set first (the star never waits on the
// network) and reconciles with the server afterwards.

import { signal } from "nsh";
import { backend } from "../services/backend";

/// A catalog model (`Model`; pbjson camelCase, `contextWindow` is a number).
export type CatalogModel = {
  id: string;
  contextWindow: number;
};

/// A provider of the catalog (`ProviderCatalogEntry`).
export type CatalogProvider = {
  name: string;
  displayName: string;
  models: CatalogModel[];
};

export type SelectedModel = {
  provider: string;
  modelId: string;
};

export function createCatalogStore(baseUrl: string) {
  const providers = signal<CatalogProvider[] | null>(null);
  const providersError = signal<string | null>(null);
  const loading = signal(false);
  const favorites = signal<Set<string>>(new Set());
  // The model new sessions run on (the desktop's SelectedModel). Not
  // persisted yet — the settings write bridge lands with the settings
  // screens.
  const selected = signal<SelectedModel | null>(null);

  function errorMessage(err: unknown): string {
    return err instanceof Error ? err.message : String(err);
  }

  /// Loads the catalog and favorites once; later calls reuse the cache
  /// (the catalog is static per server run, the desktop loads it the same
  /// way on startup).
  async function load(): Promise<void> {
    if (providers() != null || loading()) return;
    loading.set(true);
    try {
      const [catalog, favoriteModels] = await Promise.all([
        backend.providers(baseUrl),
        backend.modelFavorites(baseUrl),
      ]);
      providers.set(JSON.parse(catalog) as CatalogProvider[]);
      const favs = JSON.parse(favoriteModels) as { provider: string; modelId: string }[];
      favorites.set(new Set(favs.map((f) => `${f.provider}:${f.modelId}`)));
      providersError.set(null);
    } catch (err) {
      providers.set([]);
      providersError.set(errorMessage(err));
    } finally {
      loading.set(false);
    }
  }

  /// Toggles one favorite: optimistic local flip, then the server call,
  /// reverting to the previous state if it fails.
  async function toggleFavorite(provider: string, modelId: string): Promise<void> {
    const key = `${provider}:${modelId}`;
    const previous = favorites();
    const next = new Set(previous);
    const nowFavorite = !next.has(key);
    if (nowFavorite) next.add(key);
    else next.delete(key);
    favorites.set(next);
    try {
      await backend.setModelFavorite(baseUrl, provider, modelId, nowFavorite);
    } catch (err) {
      favorites.set(previous);
      providersError.set(errorMessage(err));
    }
  }

  function selectModel(provider: string, modelId: string): void {
    selected.set({ provider, modelId });
  }

  return {
    providers,
    providersError,
    loading,
    favorites,
    selected,
    load,
    toggleFavorite,
    selectModel,
  };
}

export type CatalogStore = ReturnType<typeof createCatalogStore>;
