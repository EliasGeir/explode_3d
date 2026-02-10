package scanner

import (
	"database/sql"
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
	tagRepo      *repository.TagRepository
	categoryRepo *repository.CategoryRepository
	settingsRepo *repository.SettingsRepository
	mu           sync.Mutex
	status       models.ScanStatus
	excludedFolders map[string]bool  // Cache for excluded folders
}

func New(rootPath string, modelRepo *repository.ModelRepository, tagRepo *repository.TagRepository, categoryRepo *repository.CategoryRepository, settingsRepo *repository.SettingsRepository) *Scanner {
	scanner := &Scanner{
		rootPath:     rootPath,
		modelRepo:    modelRepo,
		tagRepo:      tagRepo,
		categoryRepo: categoryRepo,
		settingsRepo: settingsRepo,
		excludedFolders: make(map[string]bool),
	}
	// Load excluded folders initially
	scanner.loadExcludedFolders()
	return scanner
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

// loadExcludedFolders loads the excluded folders from settings into the cache
func (s *Scanner) loadExcludedFolders() {
	s.excludedFolders = make(map[string]bool) // Reset the map
	
	if s.settingsRepo == nil {
		return
	}
	
	excludedFoldersStr, err := s.settingsRepo.Get("excluded_folders")
	if err != nil {
		return // No excluded folders set
	}
	
	if excludedFoldersStr == "" {
		return // Empty string means no exclusions
	}
	
	// Split by comma and normalize the folder names
	folderNames := strings.Split(excludedFoldersStr, ",")
	for _, folderName := range folderNames {
		folderName = strings.TrimSpace(folderName)
		if folderName != "" {
			// Convert to lowercase for case-insensitive comparison
			s.excludedFolders[strings.ToLower(folderName)] = true
		}
	}
}

// isExcludedFolder checks if a folder name is in the excluded list
func (s *Scanner) isExcludedFolder(folderName string) bool {
	if s.excludedFolders == nil {
		return false
	}
	
	// Check if the folder name (case-insensitive) is in the excluded list
	_, exists := s.excludedFolders[strings.ToLower(folderName)]
	return exists
}

// RefreshExcludedFolders reloads the excluded folders from settings
func (s *Scanner) RefreshExcludedFolders() {
	s.loadExcludedFolders()

	// Delete all categories at the start of the scan to rebuild them
	if s.categoryRepo != nil {
		if err := s.categoryRepo.DeleteAll(); err != nil {
			log.Printf("[scan] failed to clear existing categories: %v", err)
		}
	}
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
	ignoredCSV := s.settingsRepo.GetString("ignored_folder_names", defaultIgnoredFolders)
	ignoredRegex := buildIgnoredRegex(ignoredCSV)

	// Load scanner minimum depth
	minDepth := s.settingsRepo.GetInt("scanner_min_depth", 2)

	// Delete all categories at the start of the scan to rebuild them
	if s.categoryRepo != nil {
		if err := s.categoryRepo.DeleteAll(); err != nil {
			log.Printf("[scan] failed to clear existing categories: %v", err)
		}
	}

	s.setStatus(func(st *models.ScanStatus) {
		st.Message = "Scanning directories..."
	})

	s.scanDirRecursive(s.rootPath, ignoredRegex, 0, minDepth, nil)

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

func (s *Scanner) scanDirRecursive(dir string, ignoredRegex *regexp.Regexp, depth int, minDepth int, parentCategoryID *int64) {
	relPath, err := filepath.Rel(s.rootPath, dir)
	if err != nil {
		relPath = dir
	}

	// Check if this directory should be excluded from scanning
	if s.isExcludedFolder(filepath.Base(dir)) {
		log.Printf("[scan] skipping excluded folder: %s", dir)
		return
	}

	// This stores the category ID for the current directory, if it's a category.
	// This will be passed to children or models found within this directory.
	currentCategoryID := parentCategoryID

	// If current depth is less than min depth, treat as a category folder.
	// We skip the root path itself (relPath == ".").
	if s.categoryRepo != nil && relPath != "." && depth < minDepth {
		// This is a category folder, create/get it
		categoryName := filepath.Base(dir)
		categoryPath := relPath

		cat, err := s.categoryRepo.GetByPath(categoryPath)
		if err != nil && err == sql.ErrNoRows {
			// Category doesn't exist, create it
			newCat := &models.Category{
				Name:     categoryName,
				Path:     categoryPath,
				ParentID: parentCategoryID,
				Depth:    depth,
			}
			if err := s.categoryRepo.Create(newCat); err != nil {
				log.Printf("[scan] failed to create category %s: %v", categoryPath, err)
				// If we can't create a category, we can't link children, so we return.
				// This might need more graceful error handling.
				return
			}
			currentCategoryID = &newCat.ID
		} else if err != nil {
			log.Printf("[scan] failed to get category %s: %v", categoryPath, err)
			return // Skip this path
		} else {
			// Category already exists
			currentCategoryID = &cat.ID
		}
	}


	// --- Model detection logic: only applies at or past the min depth ---
	// If we are at or past minDepth, we check for models here.
	if depth >= minDepth {
		isModelDetected := false

		// Detection 1: check ONLY for direct 3D files in this directory
		directFiles := findDirect3DFiles(dir)

		if len(directFiles) > 0 && relPath != "." {
			allFiles := find3DFiles(dir, 5)
			s.processModel(dir, relPath, allFiles, currentCategoryID)
			isModelDetected = true
		}

		// Detection 2: check if subdirectories with ignored names
		// (STL, Base, LYS, 25mm, etc.) contain 3D files. If so, this directory
		// is the model, not the subdirectories.
		if !isModelDetected && relPath != "." && ignoredRegex != nil && hasIgnoredSubdirsWith3D(dir, ignoredRegex) {
			allFiles := find3DFiles(dir, 5)
			s.processModel(dir, relPath, allFiles, currentCategoryID)
			isModelDetected = true
		}

		// Detection 3: check if this directory has ANY subdirectories with 3D files
		// (even if not matching ignored patterns). This allows models that are
		// organized only in subdirectories without direct 3D files.
		if !isModelDetected && relPath != "." && hasAnySubdirWith3D(dir) {
			allFiles := find3DFiles(dir, 5)
			s.processModel(dir, relPath, allFiles, currentCategoryID)
			isModelDetected = true
		}

		if isModelDetected {
			return // If a model is detected, stop here and do not recurse further into this path
		}
	}

	// If not a model or still a category level, recurse into children
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
		s.scanDirRecursive(filepath.Join(dir, entry.Name()), ignoredRegex, depth+1, minDepth, currentCategoryID)
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

// hasAnySubdirWith3D returns true if any immediate subdirectory contains 3D files
// (recursively searched up to depth 5). This allows detecting models that are
// organized only in subdirectories without direct 3D files in the parent.
func hasAnySubdirWith3D(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subdir := filepath.Join(dir, entry.Name())
		// Check if this subdirectory or its children contain any 3D files
		if len(find3DFiles(subdir, 5)) > 0 {
			return true
		}
	}
	return false
}

func (s *Scanner) processModel(dir, relPath string, files3D []string, categoryID *int64) {
	s.setStatus(func(st *models.ScanStatus) {
		st.Processed++
		st.Message = fmt.Sprintf("Processing: %s", relPath)
	})

	existing, err := s.modelRepo.GetByPath(relPath)
	if err == nil && existing != nil {
		log.Printf("[scan] existing: %s", relPath)
		s.modelRepo.MarkScanned(existing.ID)

		// Update category if it has changed
		if existing.CategoryID == nil && categoryID != nil ||
		   existing.CategoryID != nil && categoryID != nil && *existing.CategoryID != *categoryID ||
		   existing.CategoryID != nil && categoryID == nil {
			s.modelRepo.SetCategory(existing.ID, categoryID)
			log.Printf("[scan] updated category for: %s", relPath)
		}

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
		CategoryID:    categoryID, // Assign the category ID here
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
	// Priority 1: Direct images in model dir
	if thumb := findThumbnailInDir(dir); thumb != "" {
		return thumb
	}

	// Priority 2: Images in render subdirectories (renders, imgs, etc.)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && renderDirRegex.MatchString(entry.Name()) {
			if thumb := findThumbnailInDir(filepath.Join(dir, entry.Name())); thumb != "" {
				return thumb
			}
		}
	}

	// Priority 3: Recursively search all subdirectories (max depth 3)
	return findThumbnailRecursive(dir, 0, 3)
}

// findThumbnailInDir searches for image files directly in a single directory
func findThumbnailInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if imageExts[ext] {
				return filepath.Join(dir, entry.Name())
			}
		}
	}
	return ""
}

// findThumbnailRecursive searches for image files recursively up to maxDepth
func findThumbnailRecursive(dir string, depth, maxDepth int) string {
	if depth >= maxDepth {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		subdir := filepath.Join(dir, entry.Name())

		// Check this subdirectory
		if thumb := findThumbnailInDir(subdir); thumb != "" {
			return thumb
		}

		// Recurse deeper
		if thumb := findThumbnailRecursive(subdir, depth+1, maxDepth); thumb != "" {
			return thumb
		}
	}

	return ""
}
