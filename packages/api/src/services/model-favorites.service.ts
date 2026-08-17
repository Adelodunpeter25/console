import type { ModelFavorite } from "@console/types";
import { getConsoleApiClient } from "../client";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const modelFavoritesService = {
  async list(): Promise<ModelFavorite[]> {
    const res = await getConsoleApiClient().get("/api/model-favorites");
    return unwrapData(res.data, "list model favorites");
  },

  async set(favorite: ModelFavorite, isFavorite: boolean): Promise<void> {
    const res = await getConsoleApiClient().put("/api/model-favorites", {
      ...favorite,
      favorite: isFavorite,
    });
    unwrapData(res.data, "update model favorite");
  },
};
