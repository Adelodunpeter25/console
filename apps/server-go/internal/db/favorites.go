// Model favorites. Port of agent/src/session/model-favorites.ts.
package db

import "github.com/Adelodunpeter25/console/apps/server-go/internal/types"

func (s *Storage) ListModelFavorites() ([]types.ModelFavorite, error) {
	rows, err := s.globalDB.Query(
		`SELECT provider, model_id FROM model_favorites ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.ModelFavorite, 0)
	for rows.Next() {
		var f types.ModelFavorite
		if err := rows.Scan(&f.Provider, &f.ModelID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Storage) SetModelFavorite(f types.ModelFavorite, isFavorite bool) error {
	if isFavorite {
		_, err := s.globalDB.Exec(`
			INSERT INTO model_favorites (provider, model_id, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(provider, model_id) DO NOTHING`,
			f.Provider, f.ModelID, nowMillis())
		return err
	}
	_, err := s.globalDB.Exec(
		`DELETE FROM model_favorites WHERE provider = ? AND model_id = ?`,
		f.Provider, f.ModelID)
	return err
}
