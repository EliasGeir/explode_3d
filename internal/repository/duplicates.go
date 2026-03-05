package repository

import (
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"3dmodels/internal/models"
)

// genericNames contains common subfolder names that should be skipped during detection.
var genericNames = map[string]bool{
	"stl": true, "obj": true, "lys": true, "3mf": true, "3ds": true,
	"base": true, "supported": true, "pre supported": true, "pre-supported": true,
	"unsupported": true, "presupported": true, "files": true, "parts": true,
	"print": true, "painted": true, "unpainted": true, "remix": true,
}

type DuplicateRepository struct {
	db      *sql.DB
	mu      sync.Mutex
	running bool
}

func NewDuplicateRepository(db *sql.DB) *DuplicateRepository {
	return &DuplicateRepository{db: db}
}

// IsRunning returns whether detection is currently running.
func (r *DuplicateRepository) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// RunDetection finds models with similar names and stores pairs in duplicate_pairs.
// It processes each model individually using the GIN trigram index for scalability.
// Existing dismissed pairs are preserved.
func (r *DuplicateRepository) RunDetection(threshold float64) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	log.Printf("[duplicates] starting detection with threshold %.2f", threshold)

	// Remove stale pending pairs (will be re-detected if still similar)
	if _, err := r.db.Exec(`DELETE FROM duplicate_pairs WHERE status = 'pending'`); err != nil {
		return err
	}

	// Get all non-hidden model IDs and names
	rows, err := r.db.Query(`SELECT id, name FROM models WHERE hidden = FALSE ORDER BY id`)
	if err != nil {
		return err
	}

	type modelEntry struct {
		ID   int64
		Name string
	}
	var allModels []modelEntry
	for rows.Next() {
		var m modelEntry
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			rows.Close()
			return err
		}
		allModels = append(allModels, m)
	}
	rows.Close()

	log.Printf("[duplicates] scanning %d models for similar names...", len(allModels))

	totalFound := 0
	for i, m := range allModels {
		if i%100 == 0 && i > 0 {
			log.Printf("[duplicates] progress: %d/%d models checked, %d pairs found", i, len(allModels), totalFound)
		}

		// Skip models with very short names or generic folder names
		nameLower := strings.ToLower(strings.TrimSpace(m.Name))
		if len(nameLower) < 5 || genericNames[nameLower] {
			continue
		}

		// Use similarity() directly — the % operator doesn't work reliably on all PG builds.
		// Limit to 5 most similar per model to avoid series/author domination.
		similarRows, err := r.db.Query(`
			SELECT m2.id, similarity($1, m2.name) as sim
			FROM models m2
			WHERE similarity($1, m2.name) > $3
			  AND m2.id > $2
			  AND m2.hidden = FALSE
			  AND length(m2.name) >= 5
			ORDER BY sim DESC
			LIMIT 5
		`, m.Name, m.ID, threshold)
		if err != nil {
			log.Printf("[duplicates] error querying similar models for %d: %v", m.ID, err)
			continue
		}

		for similarRows.Next() {
			var otherID int64
			var sim float64
			if err := similarRows.Scan(&otherID, &sim); err != nil {
				continue
			}

			_, err := r.db.Exec(`
				INSERT INTO duplicate_pairs (model_id_1, model_id_2, similarity, status, detected_at)
				VALUES ($1, $2, $3, 'pending', NOW())
				ON CONFLICT (model_id_1, model_id_2) DO NOTHING
			`, m.ID, otherID, sim)
			if err != nil {
				log.Printf("[duplicates] error inserting pair (%d, %d): %v", m.ID, otherID, err)
			} else {
				totalFound++
			}
		}
		similarRows.Close()
	}

	log.Printf("[duplicates] detection complete: %d pairs found across %d models", totalFound, len(allModels))
	return nil
}

// GetPendingPairs returns pending duplicate pairs with model info, paginated.
func (r *DuplicateRepository) GetPendingPairs(limit, offset int) ([]models.DuplicatePair, int, error) {
	// Get total count
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM duplicate_pairs WHERE status = 'pending'`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(`
		SELECT
			dp.id, dp.model_id_1, dp.model_id_2, dp.similarity, dp.status, dp.detected_at,
			m1.name, m1.path, COALESCE(m1.thumbnail_path, ''),
			m2.name, m2.path, COALESCE(m2.thumbnail_path, '')
		FROM duplicate_pairs dp
		JOIN models m1 ON m1.id = dp.model_id_1
		JOIN models m2 ON m2.id = dp.model_id_2
		WHERE dp.status = 'pending'
		ORDER BY dp.similarity DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pairs []models.DuplicatePair
	for rows.Next() {
		var p models.DuplicatePair
		m1 := &models.Model3D{}
		m2 := &models.Model3D{}
		if err := rows.Scan(
			&p.ID, &p.ModelID1, &p.ModelID2, &p.Similarity, &p.Status, &p.DetectedAt,
			&m1.Name, &m1.Path, &m1.ThumbnailPath,
			&m2.Name, &m2.Path, &m2.ThumbnailPath,
		); err != nil {
			return nil, 0, err
		}
		m1.ID = p.ModelID1
		m2.ID = p.ModelID2
		p.Model1 = m1
		p.Model2 = m2
		pairs = append(pairs, p)
	}
	return pairs, total, nil
}

// GetPendingCount returns the count of pending duplicate pairs.
func (r *DuplicateRepository) GetPendingCount() int {
	var count int
	r.db.QueryRow(`SELECT COUNT(*) FROM duplicate_pairs WHERE status = 'pending'`).Scan(&count)
	return count
}

// DismissPair marks a pair as "keep both" (dismissed).
func (r *DuplicateRepository) DismissPair(pairID int64, userID int64) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE duplicate_pairs
		SET status = 'dismissed', resolved_at = $1, resolved_by = $2
		WHERE id = $3`,
		now, userID, pairID)
	return err
}

// DeletePair removes a pair from the table (used when one model is deleted).
func (r *DuplicateRepository) DeletePair(pairID int64) error {
	_, err := r.db.Exec(`DELETE FROM duplicate_pairs WHERE id = $1`, pairID)
	return err
}

// GetPairByID returns a single pair.
func (r *DuplicateRepository) GetPairByID(pairID int64) (*models.DuplicatePair, error) {
	var p models.DuplicatePair
	err := r.db.QueryRow(`
		SELECT id, model_id_1, model_id_2, similarity, status, detected_at
		FROM duplicate_pairs WHERE id = $1`, pairID).Scan(
		&p.ID, &p.ModelID1, &p.ModelID2, &p.Similarity, &p.Status, &p.DetectedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePairsForModel removes all pairs involving a model (called before model deletion).
func (r *DuplicateRepository) DeletePairsForModel(modelID int64) error {
	_, err := r.db.Exec(`DELETE FROM duplicate_pairs WHERE model_id_1 = $1 OR model_id_2 = $1`, modelID)
	return err
}
