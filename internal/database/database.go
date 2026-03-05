package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(connStr string) (*sql.DB, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	log.Println("[migrate] running schema...")
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	log.Println("[migrate] schema ok")

	if _, err := db.Exec(ftsSetup); err != nil {
		return fmt.Errorf("exec fts setup: %w", err)
	}

	// Conditional migration: add scanned_at column to models if missing
	log.Println("[migrate] checking scanned_at column...")
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'models' AND column_name = 'scanned_at'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check scanned_at column: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN scanned_at TIMESTAMPTZ DEFAULT NOW()`); err != nil {
			return fmt.Errorf("add scanned_at column: %w", err)
		}
	}

	// Conditional migration: add category_id to models if missing
	log.Println("[migrate] checking category_id column...")
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'models' AND column_name = 'category_id'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check category_id column: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL`); err != nil {
			return fmt.Errorf("add category_id column: %w", err)
		}
	}

	// Conditional migration: add search_vector column if missing
	log.Println("[migrate] checking search_vector column...")
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'models' AND column_name = 'search_vector'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check search_vector column: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN search_vector TSVECTOR`); err != nil {
			return fmt.Errorf("add search_vector column: %w", err)
		}
		// Backfill existing rows
		if _, err := db.Exec(`UPDATE models SET search_vector = to_tsvector('simple', COALESCE(name, ''))`); err != nil {
			return fmt.Errorf("backfill search_vector: %w", err)
		}
	}

	// Create GIN index (IF NOT EXISTS works for indexes in PG)
	log.Println("[migrate] creating GIN index...")
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_models_search ON models USING GIN(search_vector)`); err != nil {
		return fmt.Errorf("create search index: %w", err)
	}

	// Create trigger function and trigger for search_vector
	log.Println("[migrate] creating search trigger...")
	if _, err := db.Exec(searchTrigger); err != nil {
		return fmt.Errorf("exec search trigger: %w", err)
	}

	// Conditional migration: add hidden column to models if missing
	log.Println("[migrate] checking hidden column...")
	var hiddenColCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'models' AND column_name = 'hidden'`).Scan(&hiddenColCount)
	if err != nil {
		return fmt.Errorf("check hidden column: %w", err)
	}
	if hiddenColCount == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN hidden BOOLEAN DEFAULT FALSE`); err != nil {
			return fmt.Errorf("add hidden column: %w", err)
		}
	}

	// Conditional migration: add file_format to printer_profiles if missing
	log.Println("[migrate] checking file_format column...")
	var fileFormatColCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'printer_profiles' AND column_name = 'file_format'`).Scan(&fileFormatColCount)
	if err != nil {
		return fmt.Errorf("check file_format column: %w", err)
	}
	if fileFormatColCount == 0 {
		if _, err := db.Exec(`ALTER TABLE printer_profiles ADD COLUMN file_format TEXT NOT NULL DEFAULT 'photon'`); err != nil {
			return fmt.Errorf("add file_format column: %w", err)
		}
	}

	// Seed printer profiles
	log.Println("[migrate] seeding printer profiles...")
	if err := seedPrinterProfiles(db); err != nil {
		return fmt.Errorf("seed printer profiles: %w", err)
	}
	if err := ensurePhotonUltra(db); err != nil {
		return fmt.Errorf("ensure photon ultra: %w", err)
	}

	// Seed default feedback categories if table is empty
	log.Println("[migrate] seeding feedback categories...")
	if err := seedFeedbackCategories(db); err != nil {
		return fmt.Errorf("seed feedback categories: %w", err)
	}

	// Seed default roles
	log.Println("[migrate] seeding roles...")
	if err := seedRoles(db); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}

	// Backfill: assegna ROLE_USER a tutti gli utenti senza ruoli
	log.Println("[migrate] backfilling user roles...")
	if err := backfillUserRoles(db); err != nil {
		return fmt.Errorf("backfill user roles: %w", err)
	}

	// Conditional migration: enable pg_trgm extension and create duplicate_pairs table
	log.Println("[migrate] checking pg_trgm and duplicate_pairs...")
	if _, err := db.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm`); err != nil {
		log.Printf("[migrate] warning: could not enable pg_trgm extension: %v", err)
	}

	// Create GIN trigram index on model names for fast similarity queries
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_models_name_trgm ON models USING GIN(name gin_trgm_ops)`); err != nil {
		log.Printf("[migrate] warning: could not create trigram index: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS duplicate_pairs (
			id SERIAL PRIMARY KEY,
			model_id_1 INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
			model_id_2 INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
			similarity DOUBLE PRECISION NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			detected_at TIMESTAMPTZ DEFAULT NOW(),
			resolved_at TIMESTAMPTZ,
			resolved_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			UNIQUE(model_id_1, model_id_2)
		);
		CREATE INDEX IF NOT EXISTS idx_duplicate_pairs_status ON duplicate_pairs(status);
		CREATE INDEX IF NOT EXISTS idx_duplicate_pairs_model1 ON duplicate_pairs(model_id_1);
		CREATE INDEX IF NOT EXISTS idx_duplicate_pairs_model2 ON duplicate_pairs(model_id_2);
	`); err != nil {
		log.Printf("[migrate] warning: could not create duplicate_pairs table: %v", err)
	}

	log.Println("[migrate] all migrations complete")
	return nil
}

// backfillUserRoles assegna ROLE_USER a tutti gli utenti senza ruoli e
// ROLE_ADMIN al primo utente (id minore) se non ce l'ha già.
func backfillUserRoles(db *sql.DB) error {
	// Assegna ROLE_USER a tutti gli utenti che non hanno alcun ruolo
	_, err := db.Exec(`
		INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id
		FROM users u
		CROSS JOIN roles r
		WHERE r.name = 'ROLE_USER'
		  AND NOT EXISTS (
		      SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id
		  )
	`)
	if err != nil {
		return fmt.Errorf("assign ROLE_USER to existing users: %w", err)
	}

	// Assegna ROLE_ADMIN al primo utente (id minore) se non ce l'ha già
	_, err = db.Exec(`
		INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id
		FROM (SELECT id FROM users ORDER BY id LIMIT 1) u
		CROSS JOIN roles r
		WHERE r.name = 'ROLE_ADMIN'
		  AND NOT EXISTS (
		      SELECT 1 FROM user_roles ur
		      JOIN roles ro ON ro.id = ur.role_id
		      WHERE ur.user_id = u.id AND ro.name = 'ROLE_ADMIN'
		  )
	`)
	if err != nil {
		return fmt.Errorf("assign ROLE_ADMIN to first user: %w", err)
	}

	return nil
}

func seedRoles(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT INTO roles (name, description)
		SELECT v.name, v.description
		FROM (VALUES
			('ROLE_ADMIN', 'Accesso completo'),
			('ROLE_USER',  'Accesso standard')
		) AS v(name, description)
		WHERE NOT EXISTS (SELECT 1 FROM roles WHERE name = v.name)
	`)
	return err
}

func seedPrinterProfiles(db *sql.DB) error {
	// Only seed if no profiles exist
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM printer_profiles`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type profile struct {
		name        string
		width, depth, height float64
		resX, resY  int
		pixelUM     float64
	}

	profiles := []profile{
		{"Photon Mono", 130, 80, 165, 2560, 1620, 51},
		{"Photon Mono X", 192, 120, 245, 3840, 2400, 50},
		{"Photon Mono X 6Ks", 196, 122, 200, 5760, 3600, 34},
		{"Photon Ultra", 102.4, 57.6, 165, 1280, 720, 80},
		{"Photon M3", 164, 102, 180, 4096, 2560, 40},
		{"Photon M3 Plus", 197, 122, 245, 5760, 3600, 34},
		{"Photon M3 Max", 298, 164, 300, 6480, 3600, 46},
	}

	for _, p := range profiles {
		var profileID int64
		err := db.QueryRow(`
			INSERT INTO printer_profiles (name, manufacturer, build_width_mm, build_depth_mm, build_height_mm, resolution_x, resolution_y, pixel_size_um, is_built_in)
			VALUES ($1, 'Anycubic', $2, $3, $4, $5, $6, $7, TRUE)
			RETURNING id`,
			p.name, p.width, p.depth, p.height, p.resX, p.resY, p.pixelUM,
		).Scan(&profileID)
		if err != nil {
			return fmt.Errorf("insert profile %s: %w", p.name, err)
		}

		_, err = db.Exec(`
			INSERT INTO print_settings (name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s, bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps, anti_aliasing, is_default)
			VALUES ('Default', $1, 0.05, 2.0, 30.0, 5, 6.0, 2.0, 4.0, 1, TRUE)`,
			profileID,
		)
		if err != nil {
			return fmt.Errorf("insert default settings for %s: %w", p.name, err)
		}
	}

	log.Printf("[migrate] seeded %d printer profiles with default settings", len(profiles))
	return nil
}

// ensurePhotonUltra adds the Photon Ultra profile if it doesn't exist (for DBs seeded before it was added).
func ensurePhotonUltra(db *sql.DB) error {
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM printer_profiles WHERE name = 'Photon Ultra' AND manufacturer = 'Anycubic'`).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	var profileID int64
	err := db.QueryRow(`
		INSERT INTO printer_profiles (name, manufacturer, build_width_mm, build_depth_mm, build_height_mm, resolution_x, resolution_y, pixel_size_um, is_built_in)
		VALUES ('Photon Ultra', 'Anycubic', 102.4, 57.6, 165, 1280, 720, 80, TRUE)
		RETURNING id`).Scan(&profileID)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO print_settings (name, profile_id, layer_height_mm, exposure_time_s, bottom_exposure_s, bottom_layers, lift_height_mm, lift_speed_mmps, retract_speed_mmps, anti_aliasing, is_default)
		VALUES ('Default', $1, 0.05, 2.0, 30.0, 5, 6.0, 2.0, 4.0, 1, TRUE)`, profileID)
	if err != nil {
		return err
	}

	log.Println("[migrate] added Photon Ultra profile")
	return nil
}

func seedFeedbackCategories(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT INTO feedback_categories (name, color, icon, sort_order)
		SELECT v.name, v.color, v.icon, v.sort_order
		FROM (VALUES
			('Bug', '#ef4444', '🐛', 1),
			('Suggerimento', '#6366f1', '💡', 2),
			('Domanda', '#f59e0b', '❓', 3),
			('Altro', '#6b7280', '💬', 4)
		) AS v(name, color, icon, sort_order)
		WHERE NOT EXISTS (SELECT 1 FROM feedback_categories)
	`)
	return err
}
