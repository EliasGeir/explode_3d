package repository

import (
	"database/sql"
	"strconv"
)

type SettingsRepository struct {
	db *sql.DB
}

func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(key string) (string, error) {
	var value string
	err := r.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (r *SettingsRepository) Set(key, value string) error {
	_, err := r.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

func (r *SettingsRepository) GetBool(key string, defaultVal bool) bool {
	val, err := r.Get(key)
	if err != nil {
		return defaultVal
	}
	return val == "true"
}

func (r *SettingsRepository) GetInt(key string, defaultVal int) int {
	val, err := r.Get(key)
	if err != nil {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}
