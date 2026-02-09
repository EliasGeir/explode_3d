package handlers

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"3dmodels/internal/models"
	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type ModelHandler struct {
	modelRepo  *repository.ModelRepository
	tagRepo    *repository.TagRepository
	authorRepo *repository.AuthorRepository
	scanPath   string
}

func NewModelHandler(mr *repository.ModelRepository, tr *repository.TagRepository, ar *repository.AuthorRepository, scanPath string) *ModelHandler {
	return &ModelHandler{modelRepo: mr, tagRepo: tr, authorRepo: ar, scanPath: scanPath}
}

func (h *ModelHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	authorStr := r.URL.Query().Get("author_id")
	tagStr := r.URL.Query().Get("tags")

	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	params := models.ModelListParams{
		Query:    q,
		Page:     page,
		PageSize: 24,
	}

	if authorStr != "" {
		aid, err := strconv.ParseInt(authorStr, 10, 64)
		if err == nil {
			params.AuthorID = &aid
		}
	}

	if tagStr != "" {
		for _, s := range strings.Split(tagStr, ",") {
			tid, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err == nil {
				params.TagIDs = append(params.TagIDs, tid)
			}
		}
	}

	modelList, total, err := h.modelRepo.List(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := templates.HomeData{
		Models:   modelList,
		Total:    total,
		Page:     page,
		PageSize: 24,
		Query:    q,
	}

	templates.ModelGrid(data).Render(r.Context(), w)
}

func (h *ModelHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := r.FormValue("name")
	notes := r.FormValue("notes")

	if err := h.modelRepo.Update(id, name, notes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	model, _ := h.modelRepo.GetByID(id)
	allTags, _ := h.tagRepo.GetAll()
	allAuthors, _ := h.authorRepo.GetAll()

	data := templates.ModelDetailData{
		Model:      *model,
		AllTags:    allTags,
		AllAuthors: allAuthors,
	}
	templates.ModelInfo(data).Render(r.Context(), w)
}

func (h *ModelHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	tagIDStr := r.FormValue("tag_id")
	tagID, err := strconv.ParseInt(tagIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid tag ID", http.StatusBadRequest)
		return
	}

	if err := h.modelRepo.AddTag(id, tagID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	model, _ := h.modelRepo.GetByID(id)
	allTags, _ := h.tagRepo.GetAll()
	templates.TagSection(model.Tags, allTags, id).Render(r.Context(), w)
}

func (h *ModelHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	tagIDStr := chi.URLParam(r, "tagId")
	tagID, err := strconv.ParseInt(tagIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid tag ID", http.StatusBadRequest)
		return
	}

	if err := h.modelRepo.RemoveTag(id, tagID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	model, _ := h.modelRepo.GetByID(id)
	allTags, _ := h.tagRepo.GetAll()
	templates.TagSection(model.Tags, allTags, id).Render(r.Context(), w)
}

func (h *ModelHandler) SetAuthor(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	authorIDStr := r.FormValue("author_id")

	var authorID *int64
	if authorIDStr != "" && authorIDStr != "0" {
		aid, err := strconv.ParseInt(authorIDStr, 10, 64)
		if err == nil {
			authorID = &aid
		}
	}

	if err := h.modelRepo.SetAuthor(id, authorID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	model, _ := h.modelRepo.GetByID(id)
	allAuthors, _ := h.authorRepo.GetAll()
	templates.AuthorSelect(model, allAuthors).Render(r.Context(), w)
}

func (h *ModelHandler) SearchTags(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var tags []models.Tag
	if query == "" {
		tags, _ = h.tagRepo.GetAll()
	} else {
		tags, _ = h.tagRepo.Search(query)
	}

	// Get current model tags to exclude already-assigned
	model, _ := h.modelRepo.GetByID(modelID)
	assigned := make(map[int64]bool)
	if model != nil {
		for _, t := range model.Tags {
			assigned[t.ID] = true
		}
	}

	var filtered []models.Tag
	for _, t := range tags {
		if !assigned[t.ID] {
			filtered = append(filtered, t)
		}
	}

	templates.TagTypeaheadResults(filtered, modelID, query).Render(r.Context(), w)
}

func (h *ModelHandler) AddTagByName(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Find or create the tag
	tag, err := h.tagRepo.GetByName(name)
	if err != nil {
		// Create new tag with default color
		tag, err = h.tagRepo.Create(name, "#6b7280")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Assign to model
	h.modelRepo.AddTag(modelID, tag.ID)

	model, _ := h.modelRepo.GetByID(modelID)
	allTags, _ := h.tagRepo.GetAll()
	templates.TagSection(model.Tags, allTags, modelID).Render(r.Context(), w)
}

func (h *ModelHandler) SearchAuthors(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var authors []models.Author
	if query == "" {
		authors, _ = h.authorRepo.GetAll()
	} else {
		authors, _ = h.authorRepo.Search(query)
	}

	templates.AuthorTypeaheadResults(authors, modelID, query).Render(r.Context(), w)
}

func (h *ModelHandler) SetAuthorByName(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))

	if name == "" {
		// Clear author
		h.modelRepo.SetAuthor(modelID, nil)
	} else {
		// Find or create
		author, err := h.authorRepo.GetByName(name)
		if err != nil {
			author, err = h.authorRepo.Create(name, "")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		h.modelRepo.SetAuthor(modelID, &author.ID)
	}

	model, _ := h.modelRepo.GetByID(modelID)
	allAuthors, _ := h.authorRepo.GetAll()
	templates.AuthorSection(model, allAuthors).Render(r.Context(), w)
}

func (h *ModelHandler) MergeCandidates(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	query := r.URL.Query().Get("q")

	// Get current model's tag IDs for sorting
	model, err := h.modelRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	var tagIDs []int64
	for _, t := range model.Tags {
		tagIDs = append(tagIDs, t.ID)
	}

	candidates, err := h.modelRepo.SearchForMerge(id, tagIDs, query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.MergeCandidates(candidates, id).Render(r.Context(), w)
}

func (h *ModelHandler) Merge(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	sourceIDStr := r.FormValue("source_id")
	sourceID, err := strconv.ParseInt(sourceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid source ID", http.StatusBadRequest)
		return
	}

	target, err := h.modelRepo.GetByID(targetID)
	if err != nil {
		http.Error(w, "Target model not found", http.StatusNotFound)
		return
	}

	source, err := h.modelRepo.GetByID(sourceID)
	if err != nil {
		http.Error(w, "Source model not found", http.StatusNotFound)
		return
	}

	targetDir := filepath.Join(h.scanPath, target.Path)
	sourceDir := filepath.Join(h.scanPath, source.Path)

	// Get source files from DB before moving
	sourceFiles, _ := h.modelRepo.GetFilesByModel(sourceID)

	// Build a set of source file relative paths that were skipped (identical duplicates)
	skippedRelPaths := make(map[string]bool)
	renamedFiles := make(map[string]string) // oldRelPath -> newRelPath

	// Filesystem: move files from source to target
	err = filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return nil
		}

		destPath := filepath.Join(targetDir, relPath)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(destPath), err)
		}

		destInfo, destErr := os.Stat(destPath)
		if destErr == nil {
			// Destination file exists
			srcInfo, _ := os.Stat(path)
			if srcInfo.Size() == destInfo.Size() {
				// Same name + same size → skip (duplicate)
				skippedRelPaths[relPath] = true
				return nil
			}
			// Same name + different size → rename with _merged suffix
			ext := filepath.Ext(relPath)
			base := strings.TrimSuffix(relPath, ext)
			newRelPath := base + "_merged" + ext
			destPath = filepath.Join(targetDir, newRelPath)
			renamedFiles[relPath] = newRelPath
		}

		// Move file (try rename first, fall back to copy)
		if err := moveFile(path, destPath); err != nil {
			return fmt.Errorf("move %s → %s: %w", path, destPath, err)
		}

		return nil
	})
	if err != nil {
		log.Printf("[merge] filesystem error: %v", err)
		http.Error(w, "Failed to merge files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove source directory
	os.RemoveAll(sourceDir)

	// Database operations
	// 1. Handle file records for skipped/renamed files
	for _, f := range sourceFiles {
		fileRelToSource, err := filepath.Rel(source.Path, f.FilePath)
		if err != nil {
			continue
		}
		if skippedRelPaths[fileRelToSource] {
			// Delete DB record for skipped file
			h.modelRepo.DeleteFile(f.ID)
		} else if newRel, ok := renamedFiles[fileRelToSource]; ok {
			// Update path and name for renamed file
			newFilePath := filepath.Join(target.Path, newRel)
			newFileName := filepath.Base(newRel)
			h.modelRepo.UpdateFileRecord(f.ID, newFilePath, newFileName)
		}
	}

	// 2. Update remaining source file paths to target prefix
	h.modelRepo.UpdateFilePaths(sourceID, source.Path, target.Path)

	// 3. Move file records from source to target
	h.modelRepo.MoveFiles(sourceID, targetID)

	// 4. Merge tags
	h.modelRepo.MergeTags(sourceID, targetID)

	// 5. Copy metadata (notes, author, thumbnail) if target is missing them
	h.modelRepo.CopyMetadata(sourceID, targetID)

	// 6. Delete source model
	h.modelRepo.Delete(sourceID)

	// Redirect to target model page
	w.Header().Set("HX-Redirect", fmt.Sprintf("/models/%d", targetID))
	w.WriteHeader(http.StatusOK)
}

func (h *ModelHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
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

	// Double-check: confirm param must match model name
	r.ParseForm()
	confirmName := r.FormValue("confirm_name")
	if confirmName != model.Name {
		http.Error(w, "Confirmation name does not match", http.StatusBadRequest)
		return
	}

	// Delete folder from filesystem
	modelDir := filepath.Join(h.scanPath, model.Path)
	if err := os.RemoveAll(modelDir); err != nil {
		log.Printf("[delete] failed to remove directory %s: %v", modelDir, err)
		http.Error(w, "Failed to delete model folder: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete from database
	if err := h.modelRepo.Delete(id); err != nil {
		log.Printf("[delete] failed to delete model %d from DB: %v", id, err)
		http.Error(w, "Failed to delete model from database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[delete] model %d (%s) deleted successfully", id, model.Name)

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func moveFile(src, dst string) error {
	// Try rename first (works if same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + delete
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcFile.Close()
	return os.Remove(src)
}
