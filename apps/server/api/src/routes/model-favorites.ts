import { Hono } from "hono";
import type { ModelFavorite } from "@console/types";
import { getProvider } from "../../../agent/src/commands/provider-registry.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";

export const modelFavoriteRoutes = new Hono();
const storage = new SqliteSessionStorage();

function parseFavorite(value: unknown): ModelFavorite | null {
  if (!value || typeof value !== "object") return null;
  const body = value as Record<string, unknown>;
  const provider = typeof body.provider === "string" ? getProvider(body.provider) : undefined;
  if (
    !provider ||
    typeof body.modelId !== "string" ||
    body.modelId.trim().length === 0
  ) {
    return null;
  }

  return {
    provider: provider.name,
    modelId: body.modelId,
  };
}

/** GET /api/model-favorites — List the models favorited on this backend. */
modelFavoriteRoutes.get("/model-favorites", (c) =>
  c.json({ success: true, data: storage.listModelFavorites() }),
);

/** PUT /api/model-favorites — Add or remove one model favorite. */
modelFavoriteRoutes.put("/model-favorites", async (c) => {
  const body = await c.req.json<unknown>();
  const favorite = parseFavorite(body);
  const isFavorite = body && typeof body === "object" && (body as Record<string, unknown>).favorite;

  if (!favorite || typeof isFavorite !== "boolean") {
    return c.json(
      { success: false, error: "provider, modelId, and favorite are required." },
      400,
    );
  }

  storage.setModelFavorite(favorite, isFavorite);
  return c.json({
    success: true,
    data: { ...favorite, favorite: isFavorite },
  });
});
