package repository

import (
	"database/sql"

	"3dmodels/internal/models"
)

type FavoritesRepository struct {
	db *sql.DB
}

func NewFavoritesRepository(db *sql.DB) *FavoritesRepository {
	return &FavoritesRepository{db: db}
}

func (r *FavoritesRepository) Add(userID, modelID int64) error {
	_, err := r.db.Exec(
		`INSERT INTO user_favorites (user_id, model_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, modelID,
	)
	return err
}

func (r *FavoritesRepository) Remove(userID, modelID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM user_favorites WHERE user_id = $1 AND model_id = $2`,
		userID, modelID,
	)
	return err
}

func (r *FavoritesRepository) IsFavorite(userID, modelID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM user_favorites WHERE user_id = $1 AND model_id = $2)`,
		userID, modelID,
	).Scan(&exists)
	return exists, err
}

func (r *FavoritesRepository) GetFavoriteIDs(userID int64) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT model_id FROM user_favorites WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *FavoritesRepository) GetFavoritesGrouped(userID int64) ([]models.FavoriteModel, error) {
	rows, err := r.db.Query(`
		SELECT m.id, m.name, m.thumbnail_path, COALESCE(c.name, '')
		FROM user_favorites uf
		JOIN models m ON m.id = uf.model_id
		LEFT JOIN categories c ON c.id = m.category_id
		WHERE uf.user_id = $1
		ORDER BY COALESCE(c.name, '') ASC, m.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.FavoriteModel
	for rows.Next() {
		var f models.FavoriteModel
		if err := rows.Scan(&f.ModelID, &f.ModelName, &f.ThumbnailPath, &f.CategoryName); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
