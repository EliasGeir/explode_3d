package models

import "time"

type Author struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	ParentID *int64 `json:"parent_id"`
	Depth    int    `json:"depth"`
}

type Model3D struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	AuthorID      *int64    `json:"author_id"`
	CategoryID    *int64    `json:"category_id"`
	Notes         string    `json:"notes"`
	ThumbnailPath string    `json:"thumbnail_path"`
	Hidden        bool      `json:"hidden"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ScannedAt     time.Time `json:"scanned_at"`

	// Joined fields
	Author *Author      `json:"author,omitempty"`
	Tags   []Tag        `json:"tags,omitempty"`
	Files  []ModelFile  `json:"files,omitempty"`
}

type ModelFile struct {
	ID       int64  `json:"id"`
	ModelID  int64  `json:"model_id"`
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	FileExt  string `json:"file_ext"`
	FileSize int64  `json:"file_size"`
}

type ModelGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ScanStatus struct {
	Running     bool   `json:"running"`
	Total       int    `json:"total"`
	Processed   int    `json:"processed"`
	NewModels   int    `json:"new_models"`
	Removed     int    `json:"removed"`
	Message     string `json:"message"`
}

type ModelListParams struct {
	Query      string
	TagIDs     []int64
	AuthorID   *int64
	CategoryID *int64
	Page       int
	PageSize   int
}

type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	Roles        []Role    `json:"roles,omitempty"`
}

type FeedbackCategory struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Icon      string    `json:"icon"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type Feedback struct {
	ID         int64     `json:"id"`
	UserID     *int64    `json:"user_id"`
	CategoryID *int64    `json:"category_id"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	Status     string    `json:"status"` // pending | read | resolved
	CreatedAt  time.Time `json:"created_at"`

	// Joined
	Category *FeedbackCategory `json:"category,omitempty"`
	Username string            `json:"username,omitempty"`
}
