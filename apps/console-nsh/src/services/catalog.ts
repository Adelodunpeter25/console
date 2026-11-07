// Centralized catalog and model service: loads providers, models, and favorites,
// and manages model selections and toggling favorites.

import { signal } from "nsh";
import { Console } from "console-nsh/native";
import type { CatalogProvider, SelectedModel } from "../common/model_picker";

export interface ModelFavoriteEntry {
  provider: string;
  modelId: string;
}

const providers = signal<CatalogProvider[]>([]);
const favorites = signal<Set<string>>(new Set());
const loading = signal(false);
const error = signal<string | null>(null);

let loadedBaseUrl: string | null = null;

async function refreshCatalog(baseUrl: string): Promise<void> {
  loading.set(true);
  try {
    const [rawProviders, rawFavorites] = await Promise.all([
      Console.providers(baseUrl).catch(() => ""),
      Console.modelFavorites(baseUrl).catch(() => ""),
    ]);

    if (rawProviders) {
      const parsed = JSON.parse(rawProviders) as Array<{
        name: string;
        displayName?: string;
        models?: Array<{ id: string; contextWindow?: number }>;
      }>;
      providers.set(
        parsed.map((p) => ({
          name: p.name,
          displayName: p.displayName ?? p.name,
          models: (p.models ?? []).map((m) => ({
            id: m.id,
            contextWindow: m.contextWindow ?? 0,
          })),
        })),
      );
    }

    if (rawFavorites) {
      const parsed = JSON.parse(rawFavorites) as Array<{
        provider?: string;
        modelId?: string;
      }>;
      const favSet = new Set<string>();
      for (const f of parsed) {
        if (f.provider && f.modelId) {
          favSet.add(`${f.provider}:${f.modelId}`);
        }
      }
      favorites.set(favSet);
    }
    loadedBaseUrl = baseUrl;
    error.set(null);
  } catch (err) {
    error.set(err instanceof Error ? err.message : String(err));
  } finally {
    loading.set(false);
  }
}

async function ensureCatalog(baseUrl: string): Promise<void> {
  if (loadedBaseUrl === baseUrl && providers().length > 0) return;
  return refreshCatalog(baseUrl);
}

async function toggleFavorite(
  baseUrl: string,
  provider: string,
  modelId: string,
): Promise<void> {
  const key = `${provider}:${modelId}`;
  const favs = new Set(favorites());
  const isFav = favs.has(key);
  const nextFav = !isFav;
  if (nextFav) {
    favs.add(key);
  } else {
    favs.delete(key);
  }
  favorites.set(favs);
  try {
    await Console.setModelFavorite(baseUrl, provider, modelId, nextFav);
  } catch (err) {
    if (isFav) favs.add(key);
    else favs.delete(key);
    favorites.set(favs);
    throw err;
  }
}

export const CatalogService = {
  providers: () => providers(),
  favorites: () => favorites(),
  loading: () => loading(),
  error: () => error(),
  ensure: ensureCatalog,
  refresh: refreshCatalog,
  toggleFavorite,
};
