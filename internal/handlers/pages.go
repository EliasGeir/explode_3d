package handlers

import (
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

type PageHandler struct {
	modelRepo  *repository.ModelRepository
	tagRepo    *repository.TagRepository
	authorRepo *repository.AuthorRepository
	scanPath   string
}

func NewPageHandler(mr *repository.ModelRepository, tr *repository.TagRepository, ar *repository.AuthorRepository, scanPath string) *PageHandler {
	return &PageHandler{modelRepo: mr, tagRepo: tr, authorRepo: ar, scanPath: scanPath}
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
	tags, _ := h.tagRepo.GetAllWithCount()
	authors, _ := h.authorRepo.GetAllWithCount()

	modelList, total, _ := h.modelRepo.List(models.ModelListParams{
		Page:     1,
		PageSize: 24,
	})

	data := templates.HomeData{
		Models:     modelList,
		Tags:       tags,
		Authors:    authors,
		Total:      total,
		Page:       1,
		PageSize:   24,
		Query:      "",
	}

	templates.Layout("3D Models", templates.HomePage(data)).Render(r.Context(), w)
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
	subdirs := findSubdirs(modelDir)
	log.Printf("[detail] model=%q dir=%q images=%d", model.Path, modelDir, len(images))

	data := templates.ModelDetailData{
		Model:      *model,
		AllTags:    allTags,
		AllAuthors: allAuthors,
		Images:     images,
		Subdirs:    subdirs,
	}

	templates.Layout(model.Name, templates.ModelDetailPage(data)).Render(r.Context(), w)
}

func (h *PageHandler) Authors(w http.ResponseWriter, r *http.Request) {
	authors, _ := h.authorRepo.GetAllWithCount()
	templates.Layout("Authors", templates.AuthorsPage(authors)).Render(r.Context(), w)
}

func (h *PageHandler) Tags(w http.ResponseWriter, r *http.Request) {
	tags, _ := h.tagRepo.GetAllWithCount()
	templates.Layout("Tags", templates.TagsPage(tags)).Render(r.Context(), w)
}
