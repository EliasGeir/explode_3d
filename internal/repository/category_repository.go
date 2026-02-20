package repository

import (
	"database/sql"

	"3dmodels/internal/models"
)

type CategoryRepository struct {
	db *sql.DB
}

func NewCategoryRepository(db *sql.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) GetByPath(path string) (*models.Category, error) {
	c := &models.Category{}
	var parentID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, path, parent_id, depth
		FROM categories WHERE path = $1`, path).Scan(
		&c.ID, &c.Name, &c.Path, &parentID, &c.Depth,
	)
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		c.ParentID = &parentID.Int64
	}
	return c, nil
}

func (r *CategoryRepository) Create(c *models.Category) error {
	err := r.db.QueryRow(`
		INSERT INTO categories (name, path, parent_id, depth)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		c.Name, c.Path, c.ParentID, c.Depth,
	).Scan(&c.ID)
	return err
}

func (r *CategoryRepository) GetByDepth(depth int) ([]models.Category, error) {
	rows, err := r.db.Query(`
		SELECT id, name, path, parent_id, depth
		FROM categories WHERE depth = $1 ORDER BY name ASC`, depth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Path, &parentID, &c.Depth); err == nil {
			if parentID.Valid {
				c.ParentID = &parentID.Int64
			}
			categories = append(categories, c)
		}
	}
	return categories, nil
}

func (r *CategoryRepository) GetChildren(parentID int64) ([]models.Category, error) {
	rows, err := r.db.Query(`
		SELECT id, name, path, parent_id, depth
		FROM categories WHERE parent_id = $1 ORDER BY name ASC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Path, &parentID, &c.Depth); err == nil {
			if parentID.Valid {
				c.ParentID = &parentID.Int64
			}
			categories = append(categories, c)
		}
	}
	return categories, nil
}

func (r *CategoryRepository) DeleteAll() error {
	_, err := r.db.Exec(`DELETE FROM categories`)
	return err
}

func (r *CategoryRepository) Search(query string) ([]models.Category, error) {
	rows, err := r.db.Query(`
		SELECT id, name, path, parent_id, depth
		FROM categories
		WHERE name ILIKE '%' || $1 || '%' OR path ILIKE '%' || $1 || '%'
		ORDER BY path
		LIMIT 15`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Path, &parentID, &c.Depth); err == nil {
			if parentID.Valid {
				c.ParentID = &parentID.Int64
			}
			categories = append(categories, c)
		}
	}
	return categories, nil
}

func (r *CategoryRepository) GetAll() ([]models.Category, error) {
	rows, err := r.db.Query(`
		SELECT id, name, path, parent_id, depth
		FROM categories ORDER BY path, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Path, &parentID, &c.Depth); err == nil {
			if parentID.Valid {
				c.ParentID = &parentID.Int64
			}
			categories = append(categories, c)
		}
	}
	return categories, nil
}
