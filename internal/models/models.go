package models

import (
	"time"
)

type PrinterProfile struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Manufacturer  string    `json:"manufacturer"`
	BuildWidthMM  float64   `json:"build_width_mm"`
	BuildDepthMM  float64   `json:"build_depth_mm"`
	BuildHeightMM float64   `json:"build_height_mm"`
	ResolutionX   int       `json:"resolution_x"`
	ResolutionY   int       `json:"resolution_y"`
	PixelSizeUM   float64   `json:"pixel_size_um"`
	FileFormat    string    `json:"file_format"` // "photon" or "dlp"
	IsBuiltIn     bool      `json:"is_built_in"`
	CreatedAt     time.Time `json:"created_at"`
}

type PrintSettings struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	ProfileID        int64     `json:"profile_id"`
	LayerHeightMM    float64   `json:"layer_height_mm"`
	ExposureTimeS    float64   `json:"exposure_time_s"`
	BottomExposureS  float64   `json:"bottom_exposure_s"`
	BottomLayers     int       `json:"bottom_layers"`
	LiftHeightMM     float64   `json:"lift_height_mm"`
	LiftSpeedMMPS    float64   `json:"lift_speed_mmps"`
	RetractSpeedMMPS float64   `json:"retract_speed_mmps"`
	AntiAliasing     int       `json:"anti_aliasing"`
	IsDefault        bool      `json:"is_default"`
	CreatedAt        time.Time `json:"created_at"`
}

type SliceJob struct {
	ID           string `json:"id"`
	Status       string `json:"status"` // pending, slicing, encoding, complete, error
	Progress     int    `json:"progress"`
	TotalLayers  int    `json:"total_layers"`
	CurrentLayer int    `json:"current_layer"`
	Message      string `json:"message"`
	Extension    string `json:"extension"`
	OutputPath   string `json:"-"`
}

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

type FavoriteModel struct {
	ModelID       int64
	ModelName     string
	ThumbnailPath string
	CategoryName  string // vuoto se nessuna categoria
}

type DuplicatePair struct {
	ID         int64     `json:"id"`
	ModelID1   int64     `json:"model_id_1"`
	ModelID2   int64     `json:"model_id_2"`
	Similarity float64   `json:"similarity"`
	Status     string    `json:"status"` // pending, dismissed
	DetectedAt time.Time `json:"detected_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy *int64    `json:"resolved_by,omitempty"`

	// Joined fields
	Model1 *Model3D `json:"model1,omitempty"`
	Model2 *Model3D `json:"model2,omitempty"`
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
