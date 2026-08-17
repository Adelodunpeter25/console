import type Database from "better-sqlite3";
import type { ModelFavorite } from "@console/types";

export function listModelFavorites(db: Database.Database): ModelFavorite[] {
  const rows = db
    .prepare("SELECT provider, model_id FROM model_favorites ORDER BY created_at ASC")
    .all() as Array<{ provider: ModelFavorite["provider"]; model_id: string }>;

  return rows.map((row) => ({ provider: row.provider, modelId: row.model_id }));
}

export function setModelFavorite(
  db: Database.Database,
  favorite: ModelFavorite,
  isFavorite: boolean,
): void {
  if (isFavorite) {
    db.prepare(
      `INSERT INTO model_favorites (provider, model_id, created_at)
       VALUES (?, ?, ?)
       ON CONFLICT(provider, model_id) DO NOTHING`,
    ).run(favorite.provider, favorite.modelId, Date.now());
    return;
  }

  db.prepare("DELETE FROM model_favorites WHERE provider = ? AND model_id = ?").run(
    favorite.provider,
    favorite.modelId,
  );
}
