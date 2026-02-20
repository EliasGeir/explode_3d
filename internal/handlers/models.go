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
	modelRepo    *repository.ModelRepository
	tagRepo      *repository.TagRepository
	authorRepo   *repository.AuthorRepository
	categoryRepo *repository.CategoryRepository
	scanPath     string
}

func NewModelHandler(mr *repository.ModelRepository, tr *repository.TagRepository, ar *repository.AuthorRepository, scanPath string) *ModelHandler {
	return &ModelHandler{modelRepo: mr, tagRepo: tr, authorRepo: ar, scanPath: scanPath}
}

func NewModelHandlerWithCategory(mr *repository.ModelRepository, tr *repository.TagRepository, ar *repository.AuthorRepository, cr *repository.CategoryRepository, scanPath string) *ModelHandler {
	return &ModelHandler{modelRepo: mr, tagRepo: tr, authorRepo: ar, categoryRepo: cr, scanPath: scanPath}
}

func (h *ModelHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")
	authorStr := r.URL.Query().Get("author_id")
	tagStr := r.URL.Query().Get("tags")
	categoryStr := r.URL.Query().Get("category_id")

	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize < 1 {
		pageSize = 24 // default
	} else if pageSize < 10 {
		pageSize = 10 // minimum
	} else if pageSize > 100 {
		pageSize = 100 // maximum
	}

	params := models.ModelListParams{
		Query:    q,
		Page:     page,
		PageSize: pageSize,
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

	var categoryID *int64
	if categoryStr != "" {
		cid, err := strconv.ParseInt(categoryStr, 10, 64)
		if err == nil {
			categoryID = &cid
			params.CategoryID = categoryID
		}
	}

	modelList, total, err := h.modelRepo.List(params)
	if err != nil {
		log.Printf("[List] ERROR querying models: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[List] Found %d models (page %d, pageSize %d, query=%q, categoryID=%v)",
		len(modelList), page, pageSize, q, categoryID)

	totalPages := (total + pageSize - 1) / pageSize

	data := templates.HomeData{
		Models:     modelList,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Query:      q,
		TotalPages: totalPages,
		CategoryID: categoryID,
		AuthorID:   params.AuthorID,
		TagIDs:     params.TagIDs,
	}

	if err := templates.ModelGrid(data).Render(r.Context(), w); err != nil {
		log.Printf("[List] ERROR rendering template: %v", err)
	}
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
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid target ID", http.StatusBadRequest)
		return
	}

	sourceID, err := strconv.ParseInt(r.FormValue("source_id"), 10, 64)
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

	// --- Start Transactional Merge ---
	tx, err := h.modelRepo.DB().Begin()
	if err != nil {
		log.Printf("[merge] failed to begin transaction: %v", err)
		http.Error(w, "Failed to start merge operation", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// --- 1. Prepare Paths ---
	targetAbsPath := filepath.Join(h.scanPath, target.Path)
	sourceAbsPath := filepath.Join(h.scanPath, source.Path)
	targetSubdirName := filepath.Base(target.Path)
	sourceSubdirName := filepath.Base(source.Path)

	log.Printf("[merge] target: %s, source: %s", targetAbsPath, sourceAbsPath)
	log.Printf("[merge] target subdir: %s, source subdir: %s", targetSubdirName, sourceSubdirName)

	// Defensively create the target directory if it doesn't exist
	if err := os.MkdirAll(targetAbsPath, 0755); err != nil {
		log.Printf("[merge] failed to create target directory %s: %v", targetAbsPath, err)
		http.Error(w, "Failed to create target model directory", http.StatusInternalServerError)
		return
	}

	// --- 2. Handle Target Model's own files ---
	// Move any files in the root of the target directory into its new subdirectory.
	targetRootFiles, err := getFilesInDir(targetAbsPath)
	if err != nil {
		log.Printf("[merge] failed to read target root dir %s: %v", targetAbsPath, err)
		http.Error(w, "Failed to read target model directory", http.StatusInternalServerError)
		return
	}

	if len(targetRootFiles) > 0 {
		newTargetSubdirAbs := filepath.Join(targetAbsPath, targetSubdirName)
		if err := os.MkdirAll(newTargetSubdirAbs, 0755); err != nil {
			log.Printf("[merge] failed to create target subdir %s: %v", newTargetSubdirAbs, err)
			http.Error(w, "Failed to create subdirectory for target model", http.StatusInternalServerError)
			return
		}

		for _, fileInfo := range targetRootFiles {
			oldFileAbs := filepath.Join(targetAbsPath, fileInfo.Name())
			newFileAbs := filepath.Join(newTargetSubdirAbs, fileInfo.Name())
			newFileRel := filepath.Join(target.Path, targetSubdirName, fileInfo.Name())

			// Find corresponding DB record to get its ID
			fileRecord, err := h.modelRepo.GetFileByPath(filepath.Join(target.Path, fileInfo.Name()))
			if err != nil {
				log.Printf("[merge] could not find DB record for target file %s: %v", oldFileAbs, err)
				continue // Or handle error more gracefully
			}

			// Move file and update DB
			if err := moveFile(oldFileAbs, newFileAbs); err != nil {
				log.Printf("[merge] failed to move target file %s: %v", oldFileAbs, err)
				http.Error(w, "Failed to move target model files", http.StatusInternalServerError)
				return // Triggers rollback
			}
			if err := h.modelRepo.UpdateFilePathTx(tx, fileRecord.ID, newFileRel); err != nil {
				log.Printf("[merge] failed to update DB for target file %s: %v", newFileRel, err)
				http.Error(w, "Failed to update target model file records", http.StatusInternalServerError)
				return // Triggers rollback
			}
		}
	}

	// --- 3. Handle Source Model's files ---
	sourceFiles, err := h.modelRepo.GetFilesByModel(sourceID)
	if err != nil {
		log.Printf("[merge] failed to get source files from DB for model %d: %v", sourceID, err)
		http.Error(w, "Failed to get source model files", http.StatusInternalServerError)
		return
	}

	newSourceSubdirAbs := filepath.Join(targetAbsPath, sourceSubdirName)
	if err := os.MkdirAll(newSourceSubdirAbs, 0755); err != nil {
		log.Printf("[merge] failed to create source subdir %s: %v", newSourceSubdirAbs, err)
		http.Error(w, "Failed to create subdirectory for source model", http.StatusInternalServerError)
		return
	}

	for _, fileToMove := range sourceFiles {
		oldFileAbs := filepath.Join(h.scanPath, fileToMove.FilePath)
		newFileRel := filepath.Join(target.Path, sourceSubdirName, fileToMove.FileName)
		newFileAbs := filepath.Join(h.scanPath, newFileRel)

		if err := moveFile(oldFileAbs, newFileAbs); err != nil {
			// Check if source file is missing, log and skip if so.
			if os.IsNotExist(err) {
				log.Printf("[merge] source file %s not found on disk, skipping", oldFileAbs)
				h.modelRepo.DeleteFile(fileToMove.ID) // Delete the record from DB
				continue
			}
			log.Printf("[merge] failed to move source file %s: %v", oldFileAbs, err)
			http.Error(w, "Failed to move source model files", http.StatusInternalServerError)
			return // Triggers rollback
		}
		if err := h.modelRepo.MoveFileToNewModelTx(tx, fileToMove.ID, targetID, newFileRel); err != nil {
			log.Printf("[merge] failed to update DB for source file %s: %v", newFileRel, err)
			http.Error(w, "Failed to update source model file records", http.StatusInternalServerError)
			return // Triggers rollback
		}
	}

	// --- 4. Merge Metadata and Delete Source ---
	if err := h.modelRepo.MergeTagsTx(tx, sourceID, targetID); err != nil {
		log.Printf("[merge] failed to merge tags: %v", err)
		http.Error(w, "Database error during tag merge", http.StatusInternalServerError)
		return
	}
	if err := h.modelRepo.CopyMetadataTx(tx, sourceID, targetID); err != nil {
		log.Printf("[merge] failed to copy metadata: %v", err)
		http.Error(w, "Database error during metadata copy", http.StatusInternalServerError)
		return
	}
	if err := h.modelRepo.DeleteTx(tx, sourceID); err != nil {
		log.Printf("[merge] failed to delete source model %d: %v", sourceID, err)
		http.Error(w, "Database error during source model deletion", http.StatusInternalServerError)
		return
	}

	// --- 5. Commit Transaction ---
	if err := tx.Commit(); err != nil {
		log.Printf("[merge] failed to commit transaction: %v", err)
		http.Error(w, "Failed to finalize merge", http.StatusInternalServerError)
		return
	}

	// --- 6. Filesystem Cleanup ---
	// This happens *after* commit to avoid data loss if commit fails.
	if err := os.RemoveAll(sourceAbsPath); err != nil {
		log.Printf("[merge] warning: failed to remove source directory %s: %v", sourceAbsPath, err)
	}

	log.Printf("[merge] successfully merged model %d into %d", sourceID, targetID)

	// Redirect to target model page
	w.Header().Set("HX-Redirect", fmt.Sprintf("/models/%d", targetID))
	w.WriteHeader(http.StatusOK)
}

// getFilesInDir returns a list of files (not directories) directly in a given directory.
func getFilesInDir(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}
	return files, nil
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

func (h *ModelHandler) UpdatePath(w http.ResponseWriter, r *http.Request) {
	modelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	newPath := strings.TrimSpace(r.FormValue("path"))

	if newPath == "" {
		http.Error(w, "Path cannot be empty", http.StatusBadRequest)
		return
	}

	// Get the current model
	currentModel, err := h.modelRepo.GetByID(modelID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// If the path hasn't changed, just redirect back to the model page
	if currentModel.Path == newPath {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/models/%d", modelID))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check if a model with the new path already exists
	existingModel, err := h.modelRepo.GetByPath(newPath)

	if err == nil && existingModel != nil {
		// Model with this path exists, perform merge
		// currentModel becomes the source, existingModel becomes the target
		log.Printf("[path-update] path %s already exists (model %d), merging model %d into %d",
			newPath, existingModel.ID, modelID, existingModel.ID)

		// Redirect to the merge endpoint (which will redirect to target model)
		h.performMerge(w, r, existingModel.ID, modelID)
		return
	}

	// Path doesn't exist, update the current model's path
	log.Printf("[path-update] updating model %d path from %s to %s", modelID, currentModel.Path, newPath)

	// Update model path in database
	if err := h.modelRepo.UpdatePath(modelID, newPath); err != nil {
		log.Printf("[path-update] failed to update path: %v", err)
		http.Error(w, "Failed to update path", http.StatusInternalServerError)
		return
	}

	// Update all file paths
	files, err := h.modelRepo.GetFilesByModel(modelID)
	if err != nil {
		log.Printf("[path-update] failed to get files: %v", err)
		http.Error(w, "Failed to update file paths", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		// Replace the path prefix in each file
		newFilePath := strings.Replace(file.FilePath, currentModel.Path, newPath, 1)
		if err := h.modelRepo.UpdateFileRecord(file.ID, newFilePath, filepath.Base(newFilePath)); err != nil {
			log.Printf("[path-update] failed to update file %d: %v", file.ID, err)
		}
	}

	log.Printf("[path-update] successfully updated path for model %d", modelID)

	// Redirect to the model page to reload everything with the new path
	w.Header().Set("HX-Redirect", fmt.Sprintf("/models/%d", modelID))
	w.WriteHeader(http.StatusOK)
}

func (h *ModelHandler) performMerge(w http.ResponseWriter, r *http.Request, targetID, sourceID int64) {
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

	// Start transaction
	tx, err := h.modelRepo.DB().Begin()
	if err != nil {
		log.Printf("[merge] failed to begin transaction: %v", err)
		http.Error(w, "Failed to start merge operation", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Prepare paths
	targetAbsPath := filepath.Join(h.scanPath, target.Path)
	sourceAbsPath := filepath.Join(h.scanPath, source.Path)
	targetSubdirName := filepath.Base(target.Path)
	sourceSubdirName := filepath.Base(source.Path)

	log.Printf("[merge] target: %s, source: %s", targetAbsPath, sourceAbsPath)

	// Create target directory if needed
	if err := os.MkdirAll(targetAbsPath, 0755); err != nil {
		log.Printf("[merge] failed to create target directory %s: %v", targetAbsPath, err)
		http.Error(w, "Failed to create target model directory", http.StatusInternalServerError)
		return
	}

	// Handle target model's own files
	targetRootFiles, err := getFilesInDir(targetAbsPath)
	if err != nil {
		log.Printf("[merge] failed to read target root dir %s: %v", targetAbsPath, err)
		http.Error(w, "Failed to read target model directory", http.StatusInternalServerError)
		return
	}

	if len(targetRootFiles) > 0 {
		newTargetSubdirAbs := filepath.Join(targetAbsPath, targetSubdirName)
		if err := os.MkdirAll(newTargetSubdirAbs, 0755); err != nil {
			log.Printf("[merge] failed to create target subdir %s: %v", newTargetSubdirAbs, err)
			http.Error(w, "Failed to create subdirectory for target model", http.StatusInternalServerError)
			return
		}

		for _, fileInfo := range targetRootFiles {
			oldFileAbs := filepath.Join(targetAbsPath, fileInfo.Name())
			newFileAbs := filepath.Join(newTargetSubdirAbs, fileInfo.Name())
			newFileRel := filepath.Join(target.Path, targetSubdirName, fileInfo.Name())

			fileRecord, err := h.modelRepo.GetFileByPath(filepath.Join(target.Path, fileInfo.Name()))
			if err != nil {
				log.Printf("[merge] could not find DB record for target file %s: %v", oldFileAbs, err)
				continue
			}

			if err := moveFile(oldFileAbs, newFileAbs); err != nil {
				log.Printf("[merge] failed to move target file %s: %v", oldFileAbs, err)
				http.Error(w, "Failed to move target model files", http.StatusInternalServerError)
				return
			}
			if err := h.modelRepo.UpdateFilePathTx(tx, fileRecord.ID, newFileRel); err != nil {
				log.Printf("[merge] failed to update DB for target file %s: %v", newFileRel, err)
				http.Error(w, "Failed to update target model file records", http.StatusInternalServerError)
				return
			}
		}
	}

	// Handle source model's files
	sourceFiles, err := h.modelRepo.GetFilesByModel(sourceID)
	if err != nil {
		log.Printf("[merge] failed to get source files from DB for model %d: %v", sourceID, err)
		http.Error(w, "Failed to get source model files", http.StatusInternalServerError)
		return
	}

	newSourceSubdirAbs := filepath.Join(targetAbsPath, sourceSubdirName)
	if err := os.MkdirAll(newSourceSubdirAbs, 0755); err != nil {
		log.Printf("[merge] failed to create source subdir %s: %v", newSourceSubdirAbs, err)
		http.Error(w, "Failed to create subdirectory for source model", http.StatusInternalServerError)
		return
	}

	for _, fileToMove := range sourceFiles {
		oldFileAbs := filepath.Join(h.scanPath, fileToMove.FilePath)
		newFileRel := filepath.Join(target.Path, sourceSubdirName, fileToMove.FileName)
		newFileAbs := filepath.Join(h.scanPath, newFileRel)

		if err := moveFile(oldFileAbs, newFileAbs); err != nil {
			if os.IsNotExist(err) {
				log.Printf("[merge] source file %s not found on disk, skipping", oldFileAbs)
				h.modelRepo.DeleteFile(fileToMove.ID)
				continue
			}
			log.Printf("[merge] failed to move source file %s: %v", oldFileAbs, err)
			http.Error(w, "Failed to move source model files", http.StatusInternalServerError)
			return
		}
		if err := h.modelRepo.MoveFileToNewModelTx(tx, fileToMove.ID, targetID, newFileRel); err != nil {
			log.Printf("[merge] failed to update DB for source file %s: %v", newFileRel, err)
			http.Error(w, "Failed to update source model file records", http.StatusInternalServerError)
			return
		}
	}

	// Merge metadata and delete source
	if err := h.modelRepo.MergeTagsTx(tx, sourceID, targetID); err != nil {
		log.Printf("[merge] failed to merge tags: %v", err)
		http.Error(w, "Database error during tag merge", http.StatusInternalServerError)
		return
	}
	if err := h.modelRepo.CopyMetadataTx(tx, sourceID, targetID); err != nil {
		log.Printf("[merge] failed to copy metadata: %v", err)
		http.Error(w, "Database error during metadata copy", http.StatusInternalServerError)
		return
	}
	if err := h.modelRepo.DeleteTx(tx, sourceID); err != nil {
		log.Printf("[merge] failed to delete source model %d: %v", sourceID, err)
		http.Error(w, "Database error during source model deletion", http.StatusInternalServerError)
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("[merge] failed to commit transaction: %v", err)
		http.Error(w, "Failed to finalize merge", http.StatusInternalServerError)
		return
	}

	// Filesystem cleanup
	if err := os.RemoveAll(sourceAbsPath); err != nil {
		log.Printf("[merge] warning: failed to remove source directory %s: %v", sourceAbsPath, err)
	}

	log.Printf("[merge] successfully merged model %d into %d", sourceID, targetID)

	// Redirect to target model page
	w.Header().Set("HX-Redirect", fmt.Sprintf("/models/%d", targetID))
	w.WriteHeader(http.StatusOK)
}

func (h *ModelHandler) ToggleHidden(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.modelRepo.ToggleHidden(id); err != nil {
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

func (h *ModelHandler) SetCategory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	categoryIDStr := r.FormValue("category_id")

	var categoryID *int64
	if categoryIDStr != "" && categoryIDStr != "0" {
		cid, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil {
			categoryID = &cid
		}
	}

	// Get the current model to access its current path
	currentModel, err := h.modelRepo.GetByID(modelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update the category in the database
	if err := h.modelRepo.SetCategory(modelID, categoryID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update the model's path based on the new category
	if err := h.modelRepo.UpdateModelPathForCategory(modelID, categoryID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the updated model to access its new path
	updatedModel, err := h.modelRepo.GetByID(modelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Move files from old path to new path in the filesystem
	oldPath := filepath.Join(h.scanPath, currentModel.Path)
	newPath := filepath.Join(h.scanPath, updatedModel.Path)

	// Check if the old path exists before attempting to move
	if _, err := os.Stat(oldPath); err == nil {
		// Create the new directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
			log.Printf("Failed to create directory for new path: %v", err)
			// Continue anyway, might not be needed if it's a direct path
		}

		// Move the directory/files
		if err := os.Rename(oldPath, newPath); err != nil {
			log.Printf("Failed to move model from %s to %s: %v", oldPath, newPath, err)
			// Log the error but don't return it, as the database update was successful
		} else {
			log.Printf("Moved model from %s to %s", oldPath, newPath)
		}
	}

	// Get all categories for the dropdown
	allCategories, err := h.categoryRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.CategorySelect(updatedModel, allCategories).Render(r.Context(), w)
}

func (h *ModelHandler) SearchCategories(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	model, err := h.modelRepo.GetByID(modelID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var categories []models.Category
	if query == "" {
		categories, _ = h.categoryRepo.GetAll()
	} else {
		categories, _ = h.categoryRepo.Search(query)
	}

	templates.CategoryTypeaheadResults(categories, modelID, model.CategoryID).Render(r.Context(), w)
}

func (h *ModelHandler) HideImage(w http.ResponseWriter, r *http.Request) {
	modelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	imagePath := r.FormValue("image_path")
	if imagePath == "" {
		http.Error(w, "Image path is required", http.StatusBadRequest)
		return
	}

	// Get the model to access its directory
	model, err := h.modelRepo.GetByID(modelID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// Build absolute paths
	oldAbsPath := filepath.Join(h.scanPath, imagePath)
	dir := filepath.Dir(oldAbsPath)
	filename := filepath.Base(oldAbsPath)

	// Add a dot prefix to hide the file
	newFilename := "." + filename
	newAbsPath := filepath.Join(dir, newFilename)

	// Rename the file
	if err := os.Rename(oldAbsPath, newAbsPath); err != nil {
		log.Printf("[hide-image] failed to rename %s: %v", oldAbsPath, err)
		http.Error(w, "Failed to hide image", http.StatusInternalServerError)
		return
	}

	log.Printf("[hide-image] renamed %s to %s", oldAbsPath, newAbsPath)

	// Get updated image list
	modelDir := filepath.Join(h.scanPath, model.Path)
	images := findAllImages(modelDir, h.scanPath)

	// Return updated gallery
	templates.ImageGallery(modelID, images).Render(r.Context(), w)
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
