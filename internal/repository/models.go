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
		SELECT m.id, m.name, m.path, m.author_id, m.category_id, COALESCE(m.notes, ''), COALESCE(m.thumbnail_path, ''), m.hidden, m.created_at, m.updated_at
		FROM models m WHERE m.id = $1`, id).Scan(
		&m.ID, &m.Name, &m.Path, &authorID, &categoryID, &m.Notes, &m.ThumbnailPath, &m.Hidden, &m.CreatedAt, &m.UpdatedAt,
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
		err := r.db.QueryRow(`SELECT id, name, url, created_at FROM authors WHERE id = $1`, *m.AuthorID).Scan(
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
		WHERE mt.model_id = $1`, id)
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
		FROM model_files WHERE model_id = $1`, id)
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
	argIdx := 1

	if params.Query != "" {
		// Convert search query for tsquery: split words, join with &, append :* for prefix match
		words := strings.Fields(params.Query)
		tsquery := strings.Join(words, " & ") + ":*"
		conditions = append(conditions, fmt.Sprintf("m.search_vector @@ to_tsquery('simple', $%d)", argIdx))
		args = append(args, tsquery)
		argIdx++
	}

	if params.AuthorID != nil {
		conditions = append(conditions, fmt.Sprintf("m.author_id = $%d", argIdx))
		args = append(args, *params.AuthorID)
		argIdx++
	}

	if len(params.TagIDs) > 0 {
		placeholders := make([]string, len(params.TagIDs))
		for i, tid := range params.TagIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, tid)
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf(
			"m.id IN (SELECT model_id FROM model_tags WHERE tag_id IN (%s) GROUP BY model_id HAVING COUNT(DISTINCT tag_id) = %d)",
			strings.Join(placeholders, ","), len(params.TagIDs),
		))
	}

	if params.CategoryID != nil {
		conditions = append(conditions, fmt.Sprintf(`m.category_id IN (
			WITH RECURSIVE category_tree AS (
				SELECT id FROM categories WHERE id = $%d
				UNION ALL
				SELECT c.id FROM categories c
				JOIN category_tree ct ON c.parent_id = ct.id
			)
			SELECT id FROM category_tree
		)`, argIdx))
		args = append(args, *params.CategoryID)
		argIdx++
	}

	// Exclude hidden models by default
	conditions = append(conditions, "m.hidden = FALSE")

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
		SELECT m.id, m.name, m.path, m.author_id, m.category_id, COALESCE(m.notes, ''), COALESCE(m.thumbnail_path, ''), m.hidden, m.created_at, m.updated_at
		FROM models m %s
		ORDER BY m.name ASC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

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
		if err := rows.Scan(&m.ID, &m.Name, &m.Path, &authorID, &categoryID, &m.Notes, &m.ThumbnailPath, &m.Hidden, &m.CreatedAt, &m.UpdatedAt); err != nil {
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
			WHERE mt.model_id = $1`, m.ID)
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
	err := r.db.QueryRow(`
		INSERT INTO models (name, path, author_id, category_id, notes, thumbnail_path, hidden)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		m.Name, m.Path, m.AuthorID, m.CategoryID, m.Notes, m.ThumbnailPath, m.Hidden,
	).Scan(&m.ID)
	return err
}

func (r *ModelRepository) Update(id int64, name, notes string) error {
	_, err := r.db.Exec(`
		UPDATE models SET name = $1, notes = $2, updated_at = NOW()
		WHERE id = $3`, name, notes, id)
	return err
}

func (r *ModelRepository) UpdateWithHidden(id int64, name, notes string, hidden bool) error {
	_, err := r.db.Exec(`
		UPDATE models SET name = $1, notes = $2, hidden = $3, updated_at = NOW()
		WHERE id = $4`, name, notes, hidden, id)
	return err
}

func (r *ModelRepository) UpdatePath(id int64, newPath string) error {
	_, err := r.db.Exec(`
		UPDATE models SET path = $1, updated_at = NOW()
		WHERE id = $2`, newPath, id)
	return err
}

func (r *ModelRepository) SetAuthor(modelID int64, authorID *int64) error {
	_, err := r.db.Exec(`UPDATE models SET author_id = $1, updated_at = NOW() WHERE id = $2`, authorID, modelID)
	return err
}

func (r *ModelRepository) SetCategory(modelID int64, categoryID *int64) error {
	_, err := r.db.Exec(`UPDATE models SET category_id = $1, updated_at = NOW() WHERE id = $2`, categoryID, modelID)
	return err
}

func (r *ModelRepository) GetCategoryPath(categoryID int64) (string, error) {
	var path string
	err := r.db.QueryRow(`SELECT path FROM categories WHERE id = $1`, categoryID).Scan(&path)
	return path, err
}

func (r *ModelRepository) UpdateModelPathForCategory(modelID int64, categoryID *int64) error {
	model, err := r.GetByID(modelID)
	if err != nil {
		return err
	}

	newPath := model.Path // Default to current path

	if categoryID != nil {
		categoryPath, err := r.GetCategoryPath(*categoryID)
		if err != nil {
			return err
		}
		// Construct new path: category_path/model_name
		newPath = categoryPath + "/" + model.Name
	}

	// Update the model's path in the database
	if err := r.UpdatePath(modelID, newPath); err != nil {
		return err
	}

	return nil
}

func (r *ModelRepository) AddTag(modelID, tagID int64) error {
	_, err := r.db.Exec(`INSERT INTO model_tags (model_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, modelID, tagID)
	return err
}

func (r *ModelRepository) RemoveTag(modelID, tagID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_tags WHERE model_id = $1 AND tag_id = $2`, modelID, tagID)
	return err
}

func (r *ModelRepository) GetByPath(path string) (*models.Model3D, error) {
	m := &models.Model3D{}
	var authorID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, path, author_id, COALESCE(notes, ''), COALESCE(thumbnail_path, ''), hidden, created_at, updated_at
		FROM models WHERE path = $1`, path).Scan(
		&m.ID, &m.Name, &m.Path, &authorID, &m.Notes, &m.ThumbnailPath, &m.Hidden, &m.CreatedAt, &m.UpdatedAt,
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
		FROM model_files WHERE file_path = $1`, path).Scan(
		&f.ID, &f.ModelID, &f.FilePath, &f.FileName, &f.FileExt, &f.FileSize,
	)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *ModelRepository) AddFile(f *models.ModelFile) error {
	err := r.db.QueryRow(`
		INSERT INTO model_files (model_id, file_path, file_name, file_ext, file_size)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		f.ModelID, f.FilePath, f.FileName, f.FileExt, f.FileSize,
	).Scan(&f.ID)
	return err
}

func (r *ModelRepository) DeleteFilesByModel(modelID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_files WHERE model_id = $1`, modelID)
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
	_, err := r.db.Exec(`UPDATE models SET thumbnail_path = $1 WHERE id = $2`, thumbnailPath, id)
	return err
}

func (r *ModelRepository) MarkScanned(id int64) error {
	_, err := r.db.Exec(`UPDATE models SET scanned_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *ModelRepository) DeleteStaleModels(before time.Time) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM models WHERE scanned_at < $1`, before)
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
		"DELETE FROM model_files WHERE model_id = $1",
		"DELETE FROM model_tags WHERE model_id = $1",
		"DELETE FROM models WHERE id = $1",
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
		FROM model_files WHERE model_id = $1`, modelID)
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
		UPDATE model_files SET file_path = REPLACE(file_path, $1, $2)
		WHERE model_id = $3`, oldPrefix, newPrefix, modelID)
	return err
}

func (r *ModelRepository) MoveFiles(sourceID, targetID int64) error {
	_, err := r.db.Exec(`UPDATE model_files SET model_id = $1 WHERE model_id = $2`, targetID, sourceID)
	return err
}

func (r *ModelRepository) MoveFileToModelTx(tx *sql.Tx, sourceModelID, targetModelID int64) error {
	_, err := tx.Exec(`UPDATE model_files SET model_id = $1 WHERE model_id = $2`, targetModelID, sourceModelID)
	return err
}

func (r *ModelRepository) DeleteFile(fileID int64) error {
	_, err := r.db.Exec(`DELETE FROM model_files WHERE id = $1`, fileID)
	return err
}

func (r *ModelRepository) UpdateFileRecord(fileID int64, filePath, fileName string) error {
	_, err := r.db.Exec(`UPDATE model_files SET file_path = $1, file_name = $2 WHERE id = $3`, filePath, fileName, fileID)
	return err
}

func (r *ModelRepository) UpdateFilePathTx(tx *sql.Tx, fileID int64, newRelativePath string) error {
	_, err := tx.Exec(`UPDATE model_files SET file_path = $1, file_name = $2 WHERE id = $3`,
		newRelativePath, filepath.Base(newRelativePath), fileID)
	return err
}

func (r *ModelRepository) MoveFileToNewModelTx(tx *sql.Tx, fileID, targetModelID int64, newRelativePath string) error {
	_, err := tx.Exec(`UPDATE model_files SET model_id = $1, file_path = $2, file_name = $3 WHERE id = $4`,
		targetModelID, newRelativePath, filepath.Base(newRelativePath), fileID)
	return err
}

func (r *ModelRepository) MergeTags(sourceID, targetID int64) error {
	_, err := r.db.Exec(`
		INSERT INTO model_tags (model_id, tag_id)
		SELECT $1, tag_id FROM model_tags WHERE model_id = $2
		ON CONFLICT DO NOTHING`, targetID, sourceID)
	return err
}

func (r *ModelRepository) MergeTagsTx(tx *sql.Tx, sourceID, targetID int64) error {
	_, err := tx.Exec(`
		INSERT INTO model_tags (model_id, tag_id)
		SELECT $1, tag_id FROM model_tags WHERE model_id = $2
		ON CONFLICT DO NOTHING`, targetID, sourceID)
	return err
}

func (r *ModelRepository) SearchForMerge(excludeID int64, tagIDs []int64, query string) ([]models.Model3D, error) {
	var args []interface{}
	var conditions []string
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("m.id != $%d", argIdx))
	args = append(args, excludeID)
	argIdx++

	if query != "" {
		words := strings.Fields(query)
		tsquery := strings.Join(words, " & ") + ":*"
		conditions = append(conditions, fmt.Sprintf("m.search_vector @@ to_tsquery('simple', $%d)", argIdx))
		args = append(args, tsquery)
		argIdx++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	// Order by number of shared tags (descending), then by name
	orderClause := "ORDER BY m.name ASC"
	if len(tagIDs) > 0 {
		placeholders := make([]string, len(tagIDs))
		for i, tid := range tagIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, tid)
			argIdx++
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
	_, err := r.db.Exec(`
		UPDATE models SET
			notes = CASE WHEN notes = '' THEN (SELECT notes FROM models WHERE id = $1) ELSE notes END,
			author_id = CASE WHEN author_id IS NULL THEN (SELECT author_id FROM models WHERE id = $2) ELSE author_id END,
			thumbnail_path = CASE WHEN thumbnail_path = '' THEN (SELECT thumbnail_path FROM models WHERE id = $3) ELSE thumbnail_path END,
			updated_at = NOW()
		WHERE id = $4`, sourceID, sourceID, sourceID, targetID)
	return err
}

func (r *ModelRepository) ToggleHidden(id int64) error {
	_, err := r.db.Exec(`UPDATE models SET hidden = NOT hidden, updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *ModelRepository) SetHidden(id int64, hidden bool) error {
	_, err := r.db.Exec(`UPDATE models SET hidden = $1, updated_at = NOW() WHERE id = $2`, hidden, id)
	return err
}

func (r *ModelRepository) CopyMetadataTx(tx *sql.Tx, sourceID, targetID int64) error {
	_, err := tx.Exec(`
		UPDATE models SET
			notes = CASE WHEN notes = '' THEN (SELECT notes FROM models WHERE id = $1) ELSE notes END,
			author_id = CASE WHEN author_id IS NULL THEN (SELECT author_id FROM models WHERE id = $2) ELSE author_id END,
			thumbnail_path = CASE WHEN thumbnail_path = '' THEN (SELECT thumbnail_path FROM models WHERE id = $3) ELSE thumbnail_path END,
			updated_at = NOW()
		WHERE id = $4`, sourceID, sourceID, sourceID, targetID)
	return err
}
