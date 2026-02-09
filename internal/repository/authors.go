package repository

import (
	"database/sql"

	"3dmodels/internal/models"
)

type AuthorRepository struct {
	db *sql.DB
}

func NewAuthorRepository(db *sql.DB) *AuthorRepository {
	return &AuthorRepository{db: db}
}

func (r *AuthorRepository) GetAll() ([]models.Author, error) {
	rows, err := r.db.Query(`SELECT id, name, url, created_at FROM authors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []models.Author
	for rows.Next() {
		var a models.Author
		if err := rows.Scan(&a.ID, &a.Name, &a.URL, &a.CreatedAt); err != nil {
			return nil, err
		}
		authors = append(authors, a)
	}
	return authors, nil
}

func (r *AuthorRepository) GetByID(id int64) (*models.Author, error) {
	a := &models.Author{}
	err := r.db.QueryRow(`SELECT id, name, url, created_at FROM authors WHERE id = ?`, id).Scan(
		&a.ID, &a.Name, &a.URL, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AuthorRepository) Create(name, url string) (*models.Author, error) {
	res, err := r.db.Exec(`INSERT INTO authors (name, url) VALUES (?, ?)`, name, url)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Author{ID: id, Name: name, URL: url}, nil
}

func (r *AuthorRepository) Update(id int64, name, url string) error {
	_, err := r.db.Exec(`UPDATE authors SET name = ?, url = ? WHERE id = ?`, name, url, id)
	return err
}

func (r *AuthorRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM authors WHERE id = ?`, id)
	return err
}

type AuthorWithCount struct {
	models.Author
	Count int
}

func (r *AuthorRepository) Search(query string) ([]models.Author, error) {
	rows, err := r.db.Query(`SELECT id, name, url, created_at FROM authors WHERE name LIKE ? ORDER BY name LIMIT 10`, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []models.Author
	for rows.Next() {
		var a models.Author
		if err := rows.Scan(&a.ID, &a.Name, &a.URL, &a.CreatedAt); err != nil {
			return nil, err
		}
		authors = append(authors, a)
	}
	return authors, nil
}

func (r *AuthorRepository) GetByName(name string) (*models.Author, error) {
	a := &models.Author{}
	err := r.db.QueryRow(`SELECT id, name, url, created_at FROM authors WHERE name = ? COLLATE NOCASE`, name).Scan(&a.ID, &a.Name, &a.URL, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AuthorRepository) GetAllWithCount() ([]AuthorWithCount, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.name, a.url, a.created_at, COUNT(m.id) as cnt
		FROM authors a
		LEFT JOIN models m ON m.author_id = a.id
		GROUP BY a.id
		ORDER BY a.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []AuthorWithCount
	for rows.Next() {
		var a AuthorWithCount
		if err := rows.Scan(&a.ID, &a.Name, &a.URL, &a.CreatedAt, &a.Count); err != nil {
			return nil, err
		}
		authors = append(authors, a)
	}
	return authors, nil
}
