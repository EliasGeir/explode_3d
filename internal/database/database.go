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
