package repository

import (
	"database/sql"
	"fmt"

	"3dmodels/internal/models"
)

type SlicerRepository struct {
	db *sql.DB
}

func NewSlicerRepository(db *sql.DB) *SlicerRepository {
	return &SlicerRepository{db: db}
}

// --- Printer Profiles ---

func (r *SlicerRepository) GetAllProfiles() ([]models.PrinterProfile, error) {
	rows, err := r.db.Query(`
		SELECT id, name, manufacturer, build_width_mm, build_depth_mm, build_height_mm,
		       resolution_x, resolution_y, pixel_size_um, is_built_in, created_at
		FROM printer_profiles
		ORDER BY manufacturer, name`)
	if err != nil {
		return nil, fmt.Errorf("get all profiles: %w", err)
	}
	defer rows.Close()

	var profiles []models.PrinterProfile
	for rows.Next() {
		var p models.PrinterProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.Manufacturer, &p.BuildWidthMM, &p.BuildDepthMM,
			&p.BuildHeightMM, &p.ResolutionX, &p.ResolutionY, &p.PixelSizeUM,
			&p.IsBuiltIn, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func (r *SlicerRepository) GetProfileByID(id int64) (*models.PrinterProfile, error) {
	var p models.PrinterProfile
	err := r.db.QueryRow(`
		SELECT id, name, manufacturer, build_width_mm, build_depth_mm, build_height_mm,
		       resolution_x, resolution_y, pixel_size_um, is_built_in, created_at
		FROM printer_profiles WHERE id = $1`, id).Scan(
		&p.ID, &p.Name, &p.Manufacturer, &p.BuildWidthMM, &p.BuildDepthMM,
		&p.BuildHeightMM, &p.ResolutionX, &p.ResolutionY, &p.PixelSizeUM,
		&p.IsBuiltIn, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get profile %d: %w", id, err)
	}
	return &p, nil
}

func (r *SlicerRepository) CreateProfile(p *models.PrinterProfile) error {
	return r.db.QueryRow(`
		INSERT INTO printer_profiles (name, manufacturer, build_width_mm, build_depth_mm, build_height_mm,
		                              resolution_x, resolution_y, pixel_size_um, is_built_in)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, FALSE)
		RETURNING id`,
		p.Name, p.Manufacturer, p.BuildWidthMM, p.BuildDepthMM, p.BuildHeightMM,
		p.ResolutionX, p.ResolutionY, p.PixelSizeUM,
	).Scan(&p.ID)
}

func (r *SlicerRepository) UpdateProfile(p *models.PrinterProfile) error {
	_, err := r.db.Exec(`
		UPDATE printer_profiles
		SET name = $1, manufacturer = $2, build_width_mm = $3, build_depth_mm = $4,
		    build_height_mm = $5, resolution_x = $6, resolution_y = $7, pixel_size_um = $8
		WHERE id = $9 AND is_built_in = FALSE`,
		p.Name, p.Manufacturer, p.BuildWidthMM, p.BuildDepthMM, p.BuildHeightMM,
		p.ResolutionX, p.ResolutionY, p.PixelSizeUM, p.ID)
	return err
}

func (r *SlicerRepository) DeleteProfile(id int64) error {
	_, err := r.db.Exec(`DELETE FROM printer_profiles WHERE id = $1 AND is_built_in = FALSE`, id)
	return err
}

// --- Print Settings ---

func (r *SlicerRepository) GetSettingsByProfile(profileID int64) ([]models.PrintSettings, error) {
	rows, err := r.db.Query(`
		SELECT id, name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s,
		       bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps,
		       anti_aliasing, is_default, created_at
		FROM print_settings
		WHERE profile_id = $1
		ORDER BY is_default DESC, name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("get settings by profile: %w", err)
	}
	defer rows.Close()

	var settings []models.PrintSettings
	for rows.Next() {
		var s models.PrintSettings
		if err := rows.Scan(&s.ID, &s.Name, &s.ProfileID, &s.LayerHeightMM, &s.ExposureTimeS,
			&s.BottomExposureS, &s.BottomLayers, &s.LiftHeightMM, &s.LiftSpeedMMPS,
			&s.RetractSpeedMMPS, &s.AntiAliasing, &s.IsDefault, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan settings: %w", err)
		}
		settings = append(settings, s)
	}
	return settings, nil
}

func (r *SlicerRepository) GetSettingsByID(id int64) (*models.PrintSettings, error) {
	var s models.PrintSettings
	err := r.db.QueryRow(`
		SELECT id, name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s,
		       bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps,
		       anti_aliasing, is_default, created_at
		FROM print_settings WHERE id = $1`, id).Scan(
		&s.ID, &s.Name, &s.ProfileID, &s.LayerHeightMM, &s.ExposureTimeS,
		&s.BottomExposureS, &s.BottomLayers, &s.LiftHeightMM, &s.LiftSpeedMMPS,
		&s.RetractSpeedMMPS, &s.AntiAliasing, &s.IsDefault, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get settings %d: %w", id, err)
	}
	return &s, nil
}

func (r *SlicerRepository) GetDefaultSettings(profileID int64) (*models.PrintSettings, error) {
	var s models.PrintSettings
	err := r.db.QueryRow(`
		SELECT id, name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s,
		       bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps,
		       anti_aliasing, is_default, created_at
		FROM print_settings WHERE profile_id = $1 AND is_default = TRUE
		LIMIT 1`, profileID).Scan(
		&s.ID, &s.Name, &s.ProfileID, &s.LayerHeightMM, &s.ExposureTimeS,
		&s.BottomExposureS, &s.BottomLayers, &s.LiftHeightMM, &s.LiftSpeedMMPS,
		&s.RetractSpeedMMPS, &s.AntiAliasing, &s.IsDefault, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get default settings for profile %d: %w", profileID, err)
	}
	return &s, nil
}

func (r *SlicerRepository) CreateSettings(s *models.PrintSettings) error {
	return r.db.QueryRow(`
		INSERT INTO print_settings (name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s,
		                            bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps,
		                            anti_aliasing, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		s.Name, s.ProfileID, s.LayerHeightMM, s.ExposureTimeS, s.BottomExposureS,
		s.BottomLayers, s.LiftHeightMM, s.LiftSpeedMMPS, s.RetractSpeedMMPS,
		s.AntiAliasing, s.IsDefault,
	).Scan(&s.ID)
}

func (r *SlicerRepository) UpdateSettings(s *models.PrintSettings) error {
	_, err := r.db.Exec(`
		UPDATE print_settings
		SET name = $1, layer_height_mm = $2, exposure_time_s = $3, bottom_exposure_s = $4,
		    bottom_layers = $5, lift_height_mm = $6, lift_speed_mmps = $7, retract_speed_mmps = $8,
		    anti_aliasing = $9, is_default = $10
		WHERE id = $11`,
		s.Name, s.LayerHeightMM, s.ExposureTimeS, s.BottomExposureS,
		s.BottomLayers, s.LiftHeightMM, s.LiftSpeedMMPS, s.RetractSpeedMMPS,
		s.AntiAliasing, s.IsDefault, s.ID)
	return err
}

func (r *SlicerRepository) DeleteSettings(id int64) error {
	_, err := r.db.Exec(`DELETE FROM print_settings WHERE id = $1`, id)
	return err
}
