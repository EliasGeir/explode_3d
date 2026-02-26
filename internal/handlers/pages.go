package handlers

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"3dmodels/internal/middleware"
	"3dmodels/internal/models"
	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type PageHandler struct {
	modelRepo    *repository.ModelRepository
	tagRepo      *repository.TagRepository
	authorRepo   *repository.AuthorRepository
	categoryRepo *repository.CategoryRepository
	favRepo      *repository.FavoritesRepository
	scanPath     string
}

func NewPageHandler(mr *repository.ModelRepository, tr *repository.TagRepository, ar *repository.AuthorRepository, cr *repository.CategoryRepository, favRepo *repository.FavoritesRepository, scanPath string) *PageHandler {
	return &PageHandler{modelRepo: mr, tagRepo: tr, authorRepo: ar, categoryRepo: cr, favRepo: favRepo, scanPath: scanPath}
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true, ".bmp": true,
}

func findAllImages(modelDir, scanPath string) []string {
	var images []string
	filepath.WalkDir(modelDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if imageExtensions[ext] {
			rel, err := filepath.Rel(scanPath, path)
			if err == nil {
				images = append(images, rel)
			}
		}
		return nil
	})
	return images
}

func findSubdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

func (h *PageHandler) Home(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters from query string
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	
	pageSize := 24
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			// Limit page size to reasonable values
			if ps < 10 {
				ps = 10
			} else if ps > 100 {
				ps = 100
			}
			pageSize = ps
		}
	}

	// Parse search query
	query := r.URL.Query().Get("q")

	// Parse current category ID from query string
	var currentCategoryID *int64
	if categoryStr := r.URL.Query().Get("category_id"); categoryStr != "" {
		if cid, err := strconv.ParseInt(categoryStr, 10, 64); err == nil {
			currentCategoryID = &cid
		}
	}

	// Parse author ID filter
	var authorID *int64
	if authorStr := r.URL.Query().Get("author_id"); authorStr != "" {
		if aid, err := strconv.ParseInt(authorStr, 10, 64); err == nil {
			authorID = &aid
		}
	}

	// Parse tag IDs filter
	var tagIDs []int64
	if tagsStr := r.URL.Query().Get("tags"); tagsStr != "" {
		for _, tidStr := range strings.Split(tagsStr, ",") {
			tidStr = strings.TrimSpace(tidStr)
			if tid, err := strconv.ParseInt(tidStr, 10, 64); err == nil {
				tagIDs = append(tagIDs, tid)
			}
		}
	}

	tags, _ := h.tagRepo.GetAllWithCount()
	authors, _ := h.authorRepo.GetAllWithCount()
	topLevelCategories, _ := h.categoryRepo.GetByDepth(1)

	modelListParams := models.ModelListParams{
		Page:       page,
		PageSize:   pageSize,
		Query:      query,
		CategoryID: currentCategoryID,
		AuthorID:   authorID,
		TagIDs:     tagIDs,
	}
	modelList, total, _ := h.modelRepo.List(modelListParams)

	totalPages := (total + pageSize - 1) / pageSize

	userID := middleware.GetUserID(r.Context())
	var favoriteIDs []int64
	if userID != 0 && h.favRepo != nil {
		favoriteIDs, _ = h.favRepo.GetFavoriteIDs(userID)
	}

	data := templates.HomeData{
		Models:          modelList,
		Tags:            tags,
		Authors:         authors,
		Categories:      topLevelCategories,
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
		Query:           query,
		TotalPages:      totalPages,
		CategoryID:      currentCategoryID,
		AuthorID:        authorID,
		TagIDs:          tagIDs,
		UserFavoriteIDs: favoriteIDs,
	}

	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")
	templates.LayoutWithUser(
		"3D Models",
		username,
		isAdmin,
		templates.TopCategories(data),
		nil, // No sidebar
		templates.HomePageContent(data),
	).Render(r.Context(), w)
}

func (h *PageHandler) ModelDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	model, err := h.modelRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	allTags, _ := h.tagRepo.GetAll()
	allAuthors, _ := h.authorRepo.GetAll()

	modelDir := filepath.Join(h.scanPath, model.Path)
	images := findAllImages(modelDir, h.scanPath)

	// Group files by subdirectory
	groupedFiles := make(map[string][]models.ModelFile)
	for _, file := range model.Files {
		relPath, err := filepath.Rel(model.Path, file.FilePath)
		if err != nil {
			// Should not happen, but handle gracefully
			relPath = file.FilePath
		}

		dir := filepath.Dir(relPath)
		groupName := ""
		if dir == "." {
			// This is a bit of a guess, might need adjustment.
			// The goal is to name the root group after the original folder.
			// Let's find the most common base path among all files.
			// A simpler heuristic for now:
			groupName = model.Name + " (root)"
		} else {
			groupName = dir
		}
		groupedFiles[groupName] = append(groupedFiles[groupName], file)
	}
	// A better heuristic for the root group name if there's only one group
	if len(groupedFiles) == 1 {
		for key := range groupedFiles {
			if strings.HasSuffix(key, " (root)") {
				delete(groupedFiles, key)
				groupedFiles[model.Name] = model.Files
				break
			}
		}
	}


	log.Printf("[detail] model=%q dir=%q images=%d groups=%d", model.Path, modelDir, len(images), len(groupedFiles))

	// Build back URL from query parameters
	backURL := "/"
	queryParams := r.URL.Query()

	// Build query parameters for back URL
	backParams := make(map[string]string)
	if categoryID := queryParams.Get("category_id"); categoryID != "" {
		backParams["category_id"] = categoryID
	}
	if page := queryParams.Get("page"); page != "" && page != "1" {
		backParams["page"] = page
	}
	if query := queryParams.Get("q"); query != "" {
		backParams["q"] = query
	}

	// Build URL with proper encoding
	if len(backParams) > 0 {
		first := true
		for key, value := range backParams {
			if first {
				backURL += "?"
				first = false
			} else {
				backURL += "&"
			}
			backURL += key + "=" + value
		}
	}

	allCategories, _ := h.categoryRepo.GetAll()

	detailUserID := middleware.GetUserID(r.Context())
	var favorited bool
	if detailUserID != 0 && h.favRepo != nil {
		favorited, _ = h.favRepo.IsFavorite(detailUserID, id)
	}

	data := templates.ModelDetailData{
		Model:         *model,
		AllTags:       allTags,
		AllAuthors:    allAuthors,
		AllCategories: allCategories,
		Images:        images,
		GroupedFiles:  groupedFiles,
		BackURL:       backURL,
		Favorited:     favorited,
	}

	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")
	templates.LayoutWithUser(model.Name, username, isAdmin, nil, nil, templates.ModelDetailPage(data)).Render(r.Context(), w)
}

func (h *PageHandler) Authors(w http.ResponseWriter, r *http.Request) {
	authors, _ := h.authorRepo.GetAllWithCount()
	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")
	templates.LayoutWithUser("Authors", username, isAdmin, nil, nil, templates.AuthorsPage(authors)).Render(r.Context(), w)
}

func (h *PageHandler) Tags(w http.ResponseWriter, r *http.Request) {
	tags, _ := h.tagRepo.GetAllWithCount()
	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")
	templates.LayoutWithUser("Tags", username, isAdmin, nil, nil, templates.TagsPage(tags)).Render(r.Context(), w)
}
