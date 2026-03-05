package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"3dmodels/internal/repository"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://model_3d_user:4UBVhwChHUEf@192.168.1.200:5432/model_3d?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Clean up previous test data
	db.Exec("DELETE FROM duplicate_pairs")
	fmt.Println("Cleaned up previous pairs.")

	dupRepo := repository.NewDuplicateRepository(db)

	fmt.Println("Running detection with threshold 0.6...")
	start := time.Now()
	err = dupRepo.RunDetection(0.6)
	if err != nil {
		log.Fatal("Detection failed:", err)
	}
	fmt.Printf("Detection took: %v\n", time.Since(start))

	count := dupRepo.GetPendingCount()
	fmt.Printf("Pending pairs: %d\n", count)

	pairs, total, err := dupRepo.GetPendingPairs(20, 0)
	if err != nil {
		log.Fatal("GetPendingPairs failed:", err)
	}

	fmt.Printf("\nTop 20 pairs (total: %d):\n", total)
	for _, p := range pairs {
		fmt.Printf("  %.0f%% | %s <-> %s\n", p.Similarity*100, p.Model1.Name, p.Model2.Name)
	}
}
