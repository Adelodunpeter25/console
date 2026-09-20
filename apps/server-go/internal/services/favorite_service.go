// Model favorites against the global DB.
package services

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type FavoriteService struct {
	manager *db.DB
}

func NewFavoriteService(manager *db.DB) *FavoriteService {
	return &FavoriteService{manager: manager}
}

func (s *FavoriteService) List() ([]types.ModelFavorite, error) {
	rows, err := s.manager.Global().Query(
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

func (s *FavoriteService) Set(f types.ModelFavorite, isFavorite bool) error {
	if isFavorite {
		_, err := s.manager.Global().Exec(`
			INSERT INTO model_favorites (provider, model_id, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(provider, model_id) DO NOTHING`,
			f.Provider, f.ModelID, utils.NowMillis())
		return err
	}
	_, err := s.manager.Global().Exec(
		`DELETE FROM model_favorites WHERE provider = ? AND model_id = ?`,
		f.Provider, f.ModelID)
	return err
}
