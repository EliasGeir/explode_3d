package repository

import (
	"database/sql"

	"3dmodels/internal/models"
)

type FeedbackRepository struct {
	db *sql.DB
}

func NewFeedbackRepository(db *sql.DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

// --- Categorie ---

func (r *FeedbackRepository) GetAllCategories() ([]models.FeedbackCategory, error) {
	rows, err := r.db.Query(`
		SELECT id, name, color, icon, sort_order, created_at
		FROM feedback_categories
		ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []models.FeedbackCategory
	for rows.Next() {
		var c models.FeedbackCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Icon, &c.SortOrder, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (r *FeedbackRepository) CreateCategory(name, color, icon string, sortOrder int) (*models.FeedbackCategory, error) {
	if color == "" {
		color = "#6b7280"
	}
	if icon == "" {
		icon = "💬"
	}
	var c models.FeedbackCategory
	err := r.db.QueryRow(`
		INSERT INTO feedback_categories (name, color, icon, sort_order)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, color, icon, sort_order, created_at`,
		name, color, icon, sortOrder,
	).Scan(&c.ID, &c.Name, &c.Color, &c.Icon, &c.SortOrder, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *FeedbackRepository) UpdateCategory(id int64, name, color, icon string, sortOrder int) error {
	_, err := r.db.Exec(`
		UPDATE feedback_categories SET name=$1, color=$2, icon=$3, sort_order=$4
		WHERE id=$5`,
		name, color, icon, sortOrder, id)
	return err
}

func (r *FeedbackRepository) DeleteCategory(id int64) error {
	_, err := r.db.Exec(`DELETE FROM feedback_categories WHERE id=$1`, id)
	return err
}

// --- Feedback ---

func (r *FeedbackRepository) Create(userID *int64, categoryID *int64, title, message string) (*models.Feedback, error) {
	var f models.Feedback
	err := r.db.QueryRow(`
		INSERT INTO feedbacks (user_id, category_id, title, message)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, category_id, title, message, status, created_at`,
		userID, categoryID, title, message,
	).Scan(&f.ID, &f.UserID, &f.CategoryID, &f.Title, &f.Message, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *FeedbackRepository) List(statusFilter string) ([]models.Feedback, error) {
	query := `
		SELECT f.id, f.user_id, f.category_id, f.title, f.message, f.status, f.created_at,
		       fc.id, fc.name, fc.color, fc.icon, fc.sort_order, fc.created_at,
		       u.username
		FROM feedbacks f
		LEFT JOIN feedback_categories fc ON fc.id = f.category_id
		LEFT JOIN users u ON u.id = f.user_id`

	args := []any{}
	if statusFilter != "" {
		query += ` WHERE f.status = $1`
		args = append(args, statusFilter)
	}
	query += ` ORDER BY f.created_at DESC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedbacks []models.Feedback
	for rows.Next() {
		var f models.Feedback
		var cat models.FeedbackCategory
		var catID sql.NullInt64
		var catName, catColor, catIcon sql.NullString
		var catSortOrder sql.NullInt32
		var catCreatedAt sql.NullTime
		var username sql.NullString

		err := rows.Scan(
			&f.ID, &f.UserID, &f.CategoryID, &f.Title, &f.Message, &f.Status, &f.CreatedAt,
			&catID, &catName, &catColor, &catIcon, &catSortOrder, &catCreatedAt,
			&username,
		)
		if err != nil {
			return nil, err
		}
		if catID.Valid {
			cat.ID = catID.Int64
			cat.Name = catName.String
			cat.Color = catColor.String
			cat.Icon = catIcon.String
			cat.SortOrder = int(catSortOrder.Int32)
			cat.CreatedAt = catCreatedAt.Time
			f.Category = &cat
		}
		if username.Valid {
			f.Username = username.String
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, nil
}

func (r *FeedbackRepository) UpdateStatus(id int64, status string) error {
	_, err := r.db.Exec(`UPDATE feedbacks SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (r *FeedbackRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM feedbacks WHERE id=$1`, id)
	return err
}
