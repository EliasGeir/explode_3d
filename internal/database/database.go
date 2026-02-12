package database

import (
	"database/sql"
	"fmt"

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
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	if _, err := db.Exec(ftsSetup); err != nil {
		return fmt.Errorf("exec fts setup: %w", err)
	}

	// Conditional migration: add scanned_at column to models if missing
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
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_models_search ON models USING GIN(search_vector)`); err != nil {
		return fmt.Errorf("create search index: %w", err)
	}

	// Create trigger function and trigger for search_vector
	if _, err := db.Exec(searchTrigger); err != nil {
		return fmt.Errorf("exec search trigger: %w", err)
	}

	return nil
}
