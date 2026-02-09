package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	ScanPath string
	Port     string
	DBPath   string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		ScanPath: getEnv("SCAN_PATH", "."),
		Port:     getEnv("PORT", "8080"),
		DBPath:   getEnv("DB_PATH", "./data/models.db"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
