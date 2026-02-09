package scanner

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"3dmodels/internal/models"
	"3dmodels/internal/repository"
)

var (
	model3DExts = map[string]bool{
		".stl": true, ".obj": true, ".lys": true,
		".3mf": true, ".3ds": true,
	}
	imageExts = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".webp": true, ".bmp": true,
	}
	renderDirRegex = regexp.MustCompile(`(?i)^(0?renders?|imgs?|images?|pictures?|photos?)$`)
)

const defaultIgnoredFolders = "stl,obj,3mf,lys,base,bases,part,parts,piece,pieces,supported,unsupported,presupported,pre-supported,painted,unpainted,scaled,files"

type Scanner struct {
	rootPath     string
	modelRepo    *repository.ModelRepository
	settingsRepo *repository.SettingsRepository
	mu           sync.Mutex
	status       models.ScanStatus
}

func New(rootPath string, modelRepo *repository.ModelRepository, settingsRepo *repository.SettingsRepository) *Scanner {
	return &Scanner{
		rootPath:     rootPath,
		modelRepo:    modelRepo,
		settingsRepo: settingsRepo,
	}
}

func (s *Scanner) Status() models.ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Scanner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status.Running
}

func (s *Scanner) StartScan() {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return
	}
	s.status = models.ScanStatus{Running: true, Message: "Starting scan..."}
	s.mu.Unlock()

	go s.runScan()
}

func (s *Scanner) setStatus(fn func(*models.ScanStatus)) {
	s.mu.Lock()
	fn(&s.status)
	s.mu.Unlock()
}

func buildIgnoredRegex(csv string) *regexp.Regexp {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	var parts []string
	for _, name := range strings.Split(csv, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		parts = append(parts, regexp.QuoteMeta(name))
	}
	if len(parts) == 0 {
		return nil
	}
	// Also match numeric sizes like 25mm, 32mm, etc.
	pattern := fmt.Sprintf(`(?i)^(%s|\d{2,3}\s*mm)$`, strings.Join(parts, "|"))
	re, err := regexp.Compile(pattern)
	if err != nil {
		log.Printf("[scan] invalid ignored folders regex: %v", err)
		return nil
	}
	return re
}

func DefaultIgnoredFolders() string {
	return defaultIgnoredFolders
}

func (s *Scanner) runScan() {
	scanStart := time.Now()

	defer func() {
		s.setStatus(func(st *models.ScanStatus) {
			st.Running = false
			st.Message = fmt.Sprintf("Scan complete. %d new, %d removed.", st.NewModels, st.Removed)
		})
		if s.settingsRepo != nil {
			s.settingsRepo.Set("last_scan_at", time.Now().Format(time.RFC3339))
		}
	}()

	// Load ignored folder names from settings
	ignoredCSV := defaultIgnoredFolders
	if s.settingsRepo != nil {
		if val, err := s.settingsRepo.Get("ignored_folder_names"); err == nil && val != "" {
			ignoredCSV = val
		}
	}
	ignoredRegex := buildIgnoredRegex(ignoredCSV)

	s.setStatus(func(st *models.ScanStatus) {
		st.Message = "Scanning directories..."
	})

	s.scanDirWithRegex(s.rootPath, ignoredRegex)

	// Delete stale models (not seen during this scan)
	s.setStatus(func(st *models.ScanStatus) {
		st.Message = "Cleaning up removed models..."
	})

	removed, err := s.modelRepo.DeleteStaleModels(scanStart)
	if err != nil {
		log.Printf("Error deleting stale models: %v", err)
	} else if removed > 0 {
		s.setStatus(func(st *models.ScanStatus) {
			st.Removed = int(removed)
		})
		log.Printf("Removed %d stale models", removed)
	}
}

func (s *Scanner) scanDirWithRegex(dir string, ignoredRegex *regexp.Regexp) {
	relPath, err := filepath.Rel(s.rootPath, dir)
	if err != nil {
		relPath = dir
	}

	// Detection: check ONLY for direct 3D files in this directory (depth 0)
	directFiles := findDirect3DFiles(dir)

	if len(directFiles) > 0 && relPath != "." {
		allFiles := find3DFiles(dir, 5)
		s.processModel(dir, relPath, allFiles)
		return
	}

	// No direct 3D files — check if subdirectories with ignored names
	// (STL, Base, LYS, 25mm, etc.) contain 3D files. If so, this directory
	// is the model, not the subdirectories.
	if relPath != "." && ignoredRegex != nil && hasIgnoredSubdirsWith3D(dir, ignoredRegex) {
		allFiles := find3DFiles(dir, 5)
		s.processModel(dir, relPath, allFiles)
		return
	}

	// Recurse into subdirectories (this is a category/series)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		s.scanDirWithRegex(filepath.Join(dir, entry.Name()), ignoredRegex)
	}
}

// hasIgnoredSubdirsWith3D returns true if any immediate subdirectory matches
// the ignored folder regex and contains 3D files.
func hasIgnoredSubdirsWith3D(dir string, re *regexp.Regexp) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if re.MatchString(entry.Name()) {
			if len(findDirect3DFiles(filepath.Join(dir, entry.Name()))) > 0 {
				return true
			}
		}
	}
	return false
}

func (s *Scanner) processModel(dir, relPath string, files3D []string) {
	s.setStatus(func(st *models.ScanStatus) {
		st.Processed++
		st.Message = fmt.Sprintf("Processing: %s", relPath)
	})

	existing, err := s.modelRepo.GetByPath(relPath)
	if err == nil && existing != nil {
		log.Printf("[scan] existing: %s", relPath)
		s.modelRepo.MarkScanned(existing.ID)

		if existing.ThumbnailPath == "" {
			thumbnail := findThumbnail(dir)
			if thumbnail != "" {
				thumbnailRel, _ := filepath.Rel(s.rootPath, thumbnail)
				s.modelRepo.UpdateThumbnail(existing.ID, thumbnailRel)
			}
		}
		return
	}

	log.Printf("[scan] new: %s (%d files)", relPath, len(files3D))
	thumbnail := findThumbnail(dir)
	thumbnailRel := ""
	if thumbnail != "" {
		thumbnailRel, _ = filepath.Rel(s.rootPath, thumbnail)
	}

	m := &models.Model3D{
		Name:          filepath.Base(dir),
		Path:          relPath,
		ThumbnailPath: thumbnailRel,
	}

	if err := s.modelRepo.Create(m); err != nil {
		log.Printf("Error creating model %s: %v", relPath, err)
		return
	}

	for _, f := range files3D {
		fRelPath, _ := filepath.Rel(s.rootPath, f)
		info, _ := os.Stat(f)
		var size int64
		if info != nil {
			size = info.Size()
		}
		mf := &models.ModelFile{
			ModelID:  m.ID,
			FilePath: fRelPath,
			FileName: filepath.Base(f),
			FileExt:  strings.ToLower(filepath.Ext(f)),
			FileSize: size,
		}
		if err := s.modelRepo.AddFile(mf); err != nil {
			log.Printf("Error adding file %s: %v", fRelPath, err)
		}
	}

	s.setStatus(func(st *models.ScanStatus) {
		st.NewModels++
	})
}

// findDirect3DFiles returns 3D files directly in dir (no recursion).
func findDirect3DFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if model3DExts[ext] {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func find3DFiles(dir string, maxDepth int) []string {
	var files []string
	findRecursive(dir, 0, maxDepth, &files)
	return files
}

func findRecursive(dir string, depth, maxDepth int, files *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if depth < maxDepth && !strings.HasPrefix(entry.Name(), ".") {
				findRecursive(filepath.Join(dir, entry.Name()), depth+1, maxDepth, files)
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if model3DExts[ext] {
			*files = append(*files, filepath.Join(dir, entry.Name()))
		}
	}
}

func findThumbnail(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Check direct images in model dir
	for _, entry := range entries {
		if !entry.IsDir() {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if imageExts[ext] {
				return filepath.Join(dir, entry.Name())
			}
		}
	}

	// Check render subdirectories
	for _, entry := range entries {
		if entry.IsDir() && renderDirRegex.MatchString(entry.Name()) {
			renderDir := filepath.Join(dir, entry.Name())
			renderEntries, err := os.ReadDir(renderDir)
			if err != nil {
				continue
			}
			for _, re := range renderEntries {
				if !re.IsDir() {
					ext := strings.ToLower(filepath.Ext(re.Name()))
					if imageExts[ext] {
						return filepath.Join(renderDir, re.Name())
					}
				}
			}
		}
	}

	return ""
}
