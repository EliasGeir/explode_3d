package repository

import (
	"database/sql"

	"3dmodels/internal/models"
)

type TagRepository struct {
	db *sql.DB
}

func NewTagRepository(db *sql.DB) *TagRepository {
	return &TagRepository{db: db}
}

func (r *TagRepository) GetAll() ([]models.Tag, error) {
	rows, err := r.db.Query(`SELECT id, name, color FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (r *TagRepository) GetByID(id int64) (*models.Tag, error) {
	t := &models.Tag{}
	err := r.db.QueryRow(`SELECT id, name, color FROM tags WHERE id = ?`, id).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *TagRepository) Create(name, color string) (*models.Tag, error) {
	if color == "" {
		color = "#6b7280"
	}
	res, err := r.db.Exec(`INSERT INTO tags (name, color) VALUES (?, ?)`, name, color)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Tag{ID: id, Name: name, Color: color}, nil
}

func (r *TagRepository) Update(id int64, name, color string) error {
	_, err := r.db.Exec(`UPDATE tags SET name = ?, color = ? WHERE id = ?`, name, color, id)
	return err
}

func (r *TagRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM tags WHERE id = ?`, id)
	return err
}

type TagWithCount struct {
	models.Tag
	Count int
}

func (r *TagRepository) Search(query string) ([]models.Tag, error) {
	rows, err := r.db.Query(`SELECT id, name, color FROM tags WHERE name LIKE ? ORDER BY name LIMIT 10`, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (r *TagRepository) GetByName(name string) (*models.Tag, error) {
	t := &models.Tag{}
	err := r.db.QueryRow(`SELECT id, name, color FROM tags WHERE name = ? COLLATE NOCASE`, name).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *TagRepository) GetAllWithCount() ([]TagWithCount, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.color, COUNT(mt.model_id) as cnt
		FROM tags t
		LEFT JOIN model_tags mt ON mt.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []TagWithCount
	for rows.Next() {
		var t TagWithCount
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Count); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}
