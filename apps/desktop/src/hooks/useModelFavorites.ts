import React from "react";
import type { Model, ProviderId } from "@console/types";
import { api } from "../lib/api";

const FAVORITES_STORAGE_KEY = "console.model-picker.favorites";

export function modelFavoriteKey(provider: ProviderId, modelId: string): string {
  return `${provider}:${modelId}`;
}

function readFavorites(): string[] {
  try {
    const saved = JSON.parse(localStorage.getItem(FAVORITES_STORAGE_KEY) ?? "[]");
    return Array.isArray(saved) && saved.every((item) => typeof item === "string") ? saved : [];
  } catch {
    return [];
  }
}

function favoriteFromKey(key: string) {
  const separator = key.indexOf(":");
  if (separator <= 0) return null;

  return {
    provider: key.slice(0, separator) as ProviderId,
    modelId: key.slice(separator + 1),
  };
}

/** Synchronizes model favorites with the backend while retaining offline local fallback. */
export function useModelFavorites() {
  const favoritesRef = React.useRef(readFavorites());
  const [favorites, setFavorites] = React.useState<string[]>(favoritesRef.current);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    try {
      localStorage.setItem(FAVORITES_STORAGE_KEY, JSON.stringify(favorites));
    } catch {
      // Favorites still work for the current session if storage is unavailable.
    }
  }, [favorites]);

  React.useEffect(() => {
    let disposed = false;

    void api
      .listModelFavorites()
      .then(async (savedFavorites) => {
        if (disposed) return;

        const serverFavorites = savedFavorites.map((favorite) =>
          modelFavoriteKey(favorite.provider, favorite.modelId),
        );
        const localFavorites = favoritesRef.current;

        // Migrate existing desktop-only favorites the first time the server table is empty.
        if (serverFavorites.length === 0 && localFavorites.length > 0) {
          await Promise.all(
            localFavorites.flatMap((key) => {
              const favorite = favoriteFromKey(key);
              return favorite ? [api.setModelFavorite(favorite, true)] : [];
            }),
          );
          return;
        }

        favoritesRef.current = serverFavorites;
        setFavorites(serverFavorites);
      })
      .catch(() => {
        // Keep local favorites available when the backend is offline.
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });

    return () => {
      disposed = true;
    };
  }, []);

  const toggleFavorite = React.useCallback((model: Model) => {
    const key = modelFavoriteKey(model.provider, model.id);
    const previous = favoritesRef.current;
    const isFavorite = !previous.includes(key);
    const next = isFavorite ? [...previous, key] : previous.filter((favorite) => favorite !== key);

    favoritesRef.current = next;
    setFavorites(next);

    void api.setModelFavorite({ provider: model.provider, modelId: model.id }, isFavorite).catch(() => {
      // Revert only if the user has not changed this favorite again while the request was pending.
      if (favoritesRef.current.includes(key) === isFavorite) {
        favoritesRef.current = previous;
        setFavorites(previous);
      }
    });
  }, []);

  return {
    favorites,
    favoriteSet: React.useMemo(() => new Set(favorites), [favorites]),
    loading,
    toggleFavorite,
  };
}
