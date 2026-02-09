package scanner

import (
	"context"
	"log"
	"time"

	"3dmodels/internal/repository"
)

func StartScheduler(ctx context.Context, sc *Scanner, settingsRepo *repository.SettingsRepository) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		log.Println("Scheduler started")

		for {
			select {
			case <-ctx.Done():
				log.Println("Scheduler stopped")
				return
			case <-ticker.C:
				checkAndRunScan(sc, settingsRepo)
			}
		}
	}()
}

func checkAndRunScan(sc *Scanner, settingsRepo *repository.SettingsRepository) {
	if !settingsRepo.GetBool("auto_scan_enabled", true) {
		return
	}

	scheduleHour := settingsRepo.GetInt("scan_schedule_hour", 3)
	now := time.Now()

	if now.Hour() != scheduleHour {
		return
	}

	// Check if already scanned today
	lastScanStr, err := settingsRepo.Get("last_scan_at")
	if err == nil && lastScanStr != "" {
		lastScan, err := time.Parse(time.RFC3339, lastScanStr)
		if err == nil {
			if lastScan.Year() == now.Year() && lastScan.YearDay() == now.YearDay() {
				return
			}
		}
	}

	if sc.IsRunning() {
		return
	}

	log.Printf("Scheduled scan starting (hour=%d)", scheduleHour)
	sc.StartScan()
}
