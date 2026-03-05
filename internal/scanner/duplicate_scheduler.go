package scanner

import (
	"context"
	"fmt"
	"log"
	"time"

	"3dmodels/internal/repository"
)

func StartDuplicateScheduler(ctx context.Context, dupRepo *repository.DuplicateRepository, settingsRepo *repository.SettingsRepository) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		log.Println("Duplicate detection scheduler started")

		for {
			select {
			case <-ctx.Done():
				log.Println("Duplicate detection scheduler stopped")
				return
			case <-ticker.C:
				checkAndRunDuplicateDetection(dupRepo, settingsRepo)
			}
		}
	}()
}

func checkAndRunDuplicateDetection(dupRepo *repository.DuplicateRepository, settingsRepo *repository.SettingsRepository) {
	if !settingsRepo.GetBool("duplicate_detection_enabled", false) {
		return
	}

	scheduleHour := settingsRepo.GetInt("duplicate_detection_hour", 4)
	now := time.Now()

	if now.Hour() != scheduleHour {
		return
	}

	// Check if already run today
	lastRunStr, err := settingsRepo.Get("last_duplicate_detection_at")
	if err == nil && lastRunStr != "" {
		lastRun, err := time.Parse(time.RFC3339, lastRunStr)
		if err == nil {
			if lastRun.Year() == now.Year() && lastRun.YearDay() == now.YearDay() {
				return
			}
		}
	}

	if dupRepo.IsRunning() {
		return
	}

	log.Printf("Scheduled duplicate detection starting (hour=%d)", scheduleHour)

	threshold := 0.7
	thresholdStr := settingsRepo.GetString("duplicate_similarity_threshold", "0.7")
	if t, err := parseFloat(thresholdStr); err == nil && t > 0 && t < 1 {
		threshold = t
	}

	if err := dupRepo.RunDetection(threshold); err != nil {
		log.Printf("Scheduled duplicate detection error: %v", err)
		return
	}

	settingsRepo.Set("last_duplicate_detection_at", now.Format(time.RFC3339))
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
