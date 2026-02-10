package repository

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"3dmodels/internal/models"
)

type ModelRepository struct {
	db *sql.DB
}

func NewModelRepository(db *sql.DB) *ModelRepository {
	return &ModelRepository{db: db}
}

func (r *ModelRepository) DB() *sql.DB {
	return r.db
}


func (r *ModelRepository) GetByID(id int64) (*models.Model3D, error) {
	m := &models.Model3D{}
	var authorID sql.NullInt64
	var categoryID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT m.id, m.name, m.path, m.author_id, m.category_id, COALESCE(m.notes, ''), COALESCE(m.thumbnail_path, ''), m.created_at, m.updated_at
		FROM models m WHERE m.id = ?`, id).Scan(
		&m.ID, &m.Name, &m.Path, &authorID, &categoryID, &m.Notes, &m.ThumbnailPath, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if authorID.Valid {
		m.AuthorID = &authorID.Int64
	}
	if categoryID.Valid {
		m.CategoryID = &categoryID.Int64
	}

	// Load author
	if m.AuthorID != nil {
		a := &models.Author{}
		err := r.db.QueryRow(`SELECT id, name, url, created_at FROM authors WHERE id = ?`, *m.AuthorID).Scan(
			&a.ID, &a.Name, &a.URL, &a.CreatedAt,
		)
		if err == nil {
			m.Author = a
		}
	}

	// Load tags
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.color FROM tags t
		JOIN model_tags mt ON mt.tag_id = t.id
		WHERE mt.model_id = ?`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t models.Tag
			if err := rows.Scan(&t.ID, &t.Name, &t.Color); err == nil {
				m.Tags = append(m.Tags, t)
			}
		}
	}

	// Load files
	fileRows, err := r.db.Query(`
		SELECT id, model_id, file_path, file_name, file_ext, file_size
		FROM model_files WHERE model_id = ?`, id)
	if err == nil {
		defer fileRows.Close()
		for fileRows.Next() {
			var f models.ModelFile
			if err := fileRows.Scan(&f.ID, &f.ModelID, &f.FilePath, &f.FileName, &f.FileExt, &f.FileSize); err == nil {
				m.Files = append(m.Files, f)
			}
		}
	}

	return m, nil
}

func (r *ModelRepository) List(params models.ModelListParams) ([]models.Model3D, int, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 24
	}

	var conditions []string
	var args []interface{}

	if params.Query != "" {
		conditions = append(conditions, "m.id IN (SELECT rowid FROM models_fts WHERE models_fts MATCH ?)")
		args = append(args, params.Query+"*")
	}

	if params.AuthorID != nil {
		conditions = append(conditions, "m.author_id = ?")
		args = append(args, *params.AuthorID)
	}

	if len(params.TagIDs) > 0 {
		placeholders := make([]string, len(params.TagIDs))
		for i, tid := range params.TagIDs {
			placeholders[i] = "?"
			args = append(args, tid)
		}
		conditions = append(conditions, fmt.Sprintf(
			"m.id IN (SELECT model_id FROM model_tags WHERE tag_id IN (%s) GROUP BY model_id HAVING COUNT(DISTINCT tag_id) = %d)",
			strings.Join(placeholders, ","), len(params.TagIDs),
		))
	}

	if params.CategoryID != nil {
		// Use recursive CTE to include all subcategories
		conditions = append(conditions, `m.category_id IN (
			WITH RECURSIVE category_tree AS (
				SELECT id FROM categories WHERE id = ?
				UNION ALL
				SELECT c.id FROM categories c
				JOIN category_tree ct ON c.parent_id = ct.id
			)
			SELECT id FROM category_tree
		)`)
		args = append(args, *params.CategoryID)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM models m %s", where)
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch page
	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT m.id, m.name, m.path, m.author_id, m.category_id, COALESCE(m.notes, ''), COALESCE(m.thumbnail_path, ''), m.created_at, m.updated_at
		FROM models m %s
		ORDER BY m.name ASC
		LIMIT ? OFFSET ?`, where)

	queryArgs := append(args, params.PageSize, offset)
	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []models.Model3D
	for rows.Next() {
		var m models.Model3D
		var authorID sql.NullInt64
		var categoryID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Name, &m.Path, &authorID, &categoryID, &m.Notes, &m.ThumbnailPath, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if authorID.Valid {
			m.AuthorID = &authorID.Int64
		}
		if categoryID.Valid {
			m.CategoryID = &categoryID.Int64
		}

		// Load tags for each model
		tagRows, err := r.db.Query(`
			SELECT t.id, t.name, t.color FROM tags t
			JOIN model_tags mt ON mt.tag_id = t.id
			WHERE mt.model_id = ?`, m.ID)
		if err == nil {
			for tagRows.Next() {
				var t models.Tag
				if err := tagRows.Scan(&t.ID, &t.Name, &t.Color); err == nil {
					m.Tags = append(m.Tags, t)
				}
			}
			tagRows.Close()
		}

		result = append(result, m)
	}

	return result, total, nil
}

func (r *ModelRepository) Create(m *models.Model3D) error {
	res, err := r.db.Exec(`
		INSERT INTO models (name, path, author_id, category_id, notes, thumbnail_path)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.Name, m.Path, m.AuthorID, m.CategoryID, m.Notes, m.ThumbnailPath,
	)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

func (r *ModelRepository) Update(id int64, name, notes string) error {
	_, err := r.db.Exec(`
		UPDATE models SET name = ?, notes = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, name, notes, id)
	return err
}

func (r *ModelRepository) UpdatePath(id int64, newPath string) error {
	_, err := r.db.Exec(`
		UPDATE models SET path = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, newPath, id)
	return err
}

func (r *ModelRepository) SetAuthor(modelID int64, authorID *int64) error {
	_, err := r.db.Exec(`UPDATE models SET author_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, authorID, modelID)
	return err
}

func (r *ModelRepository) SetCategory(modelID int64, categoryID *int64) error {
	_, err := r.db.Exec(`UPDATE models SET category_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, categoryID, modelID)
	return err
}

func (r *ModelRepository) AddTag(modelID, tagID int64) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO model_tags (model_id, tag_id) VALUES (?, ?)`, modelID, tagID)
	return err
}

func (r *ModelRepository) RemoveTag(modelID, tagID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_tags WHERE model_id = ? AND tag_id = ?`, modelID, tagID)
	return err
}

func (r *ModelRepository) GetByPath(path string) (*models.Model3D, error) {
	m := &models.Model3D{}
	var authorID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, path, author_id, COALESCE(notes, ''), COALESCE(thumbnail_path, ''), created_at, updated_at
		FROM models WHERE path = ?`, path).Scan(
		&m.ID, &m.Name, &m.Path, &authorID, &m.Notes, &m.ThumbnailPath, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if authorID.Valid {
		m.AuthorID = &authorID.Int64
	}
	return m, nil
}

func (r *ModelRepository) GetFileByPath(path string) (*models.ModelFile, error) {
	f := &models.ModelFile{}
	err := r.db.QueryRow(`
		SELECT id, model_id, file_path, file_name, file_ext, file_size
		FROM model_files WHERE file_path = ?`, path).Scan(
		&f.ID, &f.ModelID, &f.FilePath, &f.FileName, &f.FileExt, &f.FileSize,
	)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *ModelRepository) AddFile(f *models.ModelFile) error {
	res, err := r.db.Exec(`
		INSERT INTO model_files (model_id, file_path, file_name, file_ext, file_size)
		VALUES (?, ?, ?, ?, ?)`,
		f.ModelID, f.FilePath, f.FileName, f.FileExt, f.FileSize,
	)
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

func (r *ModelRepository) DeleteFilesByModel(modelID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_files WHERE model_id = ?`, modelID)
	return err
}

func (r *ModelRepository) AllPaths() (map[string]bool, error) {
	rows, err := r.db.Query(`SELECT path FROM models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]bool)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths[p] = true
		}
	}
	return paths, nil
}

func (r *ModelRepository) UpdateThumbnail(id int64, thumbnailPath string) error {
	_, err := r.db.Exec(`UPDATE models SET thumbnail_path = ? WHERE id = ?`, thumbnailPath, id)
	return err
}

func (r *ModelRepository) MarkScanned(id int64) error {
	_, err := r.db.Exec(`UPDATE models SET scanned_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (r *ModelRepository) DeleteStaleModels(before time.Time) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM models WHERE scanned_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *ModelRepository) Delete(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.DeleteTx(tx, id); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ModelRepository) DeleteTx(tx *sql.Tx, id int64) error {
	for _, q := range []string{
		"DELETE FROM model_files WHERE model_id = ?",
		"DELETE FROM model_tags WHERE model_id = ?",
		"DELETE FROM models WHERE id = ?",
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return nil
}


func (r *ModelRepository) GetFilesByModel(modelID int64) ([]models.ModelFile, error) {
	rows, err := r.db.Query(`
		SELECT id, model_id, file_path, file_name, file_ext, file_size
		FROM model_files WHERE model_id = ?`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []models.ModelFile
	for rows.Next() {
		var f models.ModelFile
		if err := rows.Scan(&f.ID, &f.ModelID, &f.FilePath, &f.FileName, &f.FileExt, &f.FileSize); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func (r *ModelRepository) UpdateFilePaths(modelID int64, oldPrefix, newPrefix string) error {
	_, err := r.db.Exec(`
		UPDATE model_files SET file_path = REPLACE(file_path, ?, ?)
		WHERE model_id = ?`, oldPrefix, newPrefix, modelID)
	return err
}

func (r *ModelRepository) MoveFiles(sourceID, targetID int64) error {
	_, err := r.db.Exec(`UPDATE model_files SET model_id = ? WHERE model_id = ?`, targetID, sourceID)
	return err
}

func (r *ModelRepository) DeleteFile(fileID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_files WHERE id = ?`, fileID)
	return err
}

func (r *ModelRepository) UpdateFileRecord(fileID int64, filePath, fileName string) error {
	_, err := r.db.Exec(`UPDATE model_files SET file_path = ?, file_name = ? WHERE id = ?`, filePath, fileName, fileID)
	return err
}

func (r *ModelRepository) UpdateFilePathTx(tx *sql.Tx, fileID int64, newRelativePath string) error {
	_, err := tx.Exec(`UPDATE model_files SET file_path = ?, file_name = ? WHERE id = ?`,
		newRelativePath, filepath.Base(newRelativePath), fileID)
	return err
}

func (r *ModelRepository) MoveFileToNewModelTx(tx *sql.Tx, fileID, targetModelID int64, newRelativePath string) error {
	_, err := tx.Exec(`UPDATE model_files SET model_id = ?, file_path = ?, file_name = ? WHERE id = ?`,
		targetModelID, newRelativePath, filepath.Base(newRelativePath), fileID)
	return err
}

func (r *ModelRepository) MergeTags(sourceID, targetID int64) error {
	// Copy tags from source to target (ignore duplicates)
	_, err := r.db.Exec(`
		INSERT OR IGNORE INTO model_tags (model_id, tag_id)
		SELECT ?, tag_id FROM model_tags WHERE model_id = ?`, targetID, sourceID)
	return err
}

func (r *ModelRepository) MergeTagsTx(tx *sql.Tx, sourceID, targetID int64) error {
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO model_tags (model_id, tag_id)
		SELECT ?, tag_id FROM model_tags WHERE model_id = ?`, targetID, sourceID)
	return err
}

func (r *ModelRepository) SearchForMerge(excludeID int64, tagIDs []int64, query string) ([]models.Model3D, error) {
	// Build a query that returns models with shared tags first, then others
	var args []interface{}
	var conditions []string

	conditions = append(conditions, "m.id != ?")
	args = append(args, excludeID)

	if query != "" {
		conditions = append(conditions, "m.id IN (SELECT rowid FROM models_fts WHERE models_fts MATCH ?)")
		args = append(args, query+"*")
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	// Order by number of shared tags (descending), then by name
	orderClause := "ORDER BY m.name ASC"
	if len(tagIDs) > 0 {
		placeholders := make([]string, len(tagIDs))
		for i, tid := range tagIDs {
			placeholders[i] = "?"
			args = append(args, tid)
		}
		orderClause = fmt.Sprintf(`ORDER BY (
			SELECT COUNT(*) FROM model_tags mt2
			WHERE mt2.model_id = m.id AND mt2.tag_id IN (%s)
		) DESC, m.name ASC`, strings.Join(placeholders, ","))
	}

	q := fmt.Sprintf(`
		SELECT m.id, m.name, m.path, COALESCE(m.thumbnail_path, '')
		FROM models m
		%s
		%s
		LIMIT 20`, where, orderClause)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Model3D
	for rows.Next() {
		var m models.Model3D
		if err := rows.Scan(&m.ID, &m.Name, &m.Path, &m.ThumbnailPath); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

func (r *ModelRepository) CopyMetadata(sourceID, targetID int64) error {
	// Copy notes and author from source if target is missing them
	_, err := r.db.Exec(`
		UPDATE models SET
			notes = CASE WHEN notes = '' THEN (SELECT notes FROM models WHERE id = ?) ELSE notes END,
			author_id = CASE WHEN author_id IS NULL THEN (SELECT author_id FROM models WHERE id = ?) ELSE author_id END,
			thumbnail_path = CASE WHEN thumbnail_path = '' THEN (SELECT thumbnail_path FROM models WHERE id = ?) ELSE thumbnail_path END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, sourceID, sourceID, sourceID, targetID)
	return err
}

func (r *ModelRepository) CopyMetadataTx(tx *sql.Tx, sourceID, targetID int64) error {
	// Copy notes and author from source if target is missing them
	_, err := tx.Exec(`
		UPDATE models SET
			notes = CASE WHEN notes = '' THEN (SELECT notes FROM models WHERE id = ?) ELSE notes END,
			author_id = CASE WHEN author_id IS NULL THEN (SELECT author_id FROM models WHERE id = ?) ELSE author_id END,
			thumbnail_path = CASE WHEN thumbnail_path = '' THEN (SELECT thumbnail_path FROM models WHERE id = ?) ELSE thumbnail_path END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, sourceID, sourceID, sourceID, targetID)
	return err
}
