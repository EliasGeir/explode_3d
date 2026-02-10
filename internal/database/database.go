package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_fk=1")
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
	if _, err := db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("exec fts schema: %w", err)
	}
	if _, err := db.Exec(ftsTriggers); err != nil {
		return fmt.Errorf("exec fts triggers: %w", err)
	}

	// Conditional migration: add scanned_at column to models if missing
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('models') WHERE name = 'scanned_at'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check scanned_at column: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN scanned_at DATETIME DEFAULT CURRENT_TIMESTAMP`); err != nil {
			return fmt.Errorf("add scanned_at column: %w", err)
		}
	}

	// Conditional migration: add category_id to models if missing
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('models') WHERE name = 'category_id'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check category_id column: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE models ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL`); err != nil {
			return fmt.Errorf("add category_id column: %w", err)
		}
	}

	return nil
}
