package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"3dmodels/internal/config"
	"3dmodels/internal/database"
	"3dmodels/internal/handlers"
	authmw "3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/internal/scanner"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := database.Open(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	modelRepo := repository.NewModelRepository(db)
	tagRepo := repository.NewTagRepository(db)
	authorRepo := repository.NewAuthorRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	userRepo := repository.NewUserRepository(db)

	sc := scanner.New(cfg.ScanPath, modelRepo, tagRepo, categoryRepo, settingsRepo)

	// Start scheduler
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	scanner.StartScheduler(ctx, sc, settingsRepo)

	pageHandler := handlers.NewPageHandler(modelRepo, tagRepo, authorRepo, categoryRepo, cfg.ScanPath)
	modelHandler := handlers.NewModelHandlerWithCategory(modelRepo, tagRepo, authorRepo, categoryRepo, sc, cfg.ScanPath)
	tagHandler := handlers.NewTagHandler(tagRepo)
	authorHandler := handlers.NewAuthorHandler(authorRepo)
	scanHandler := handlers.NewScanHandler(sc)
	settingsHandler := handlers.NewSettingsHandler(settingsRepo, sc, userRepo)
	categoryHandler := handlers.NewCategoryHandler(categoryRepo)
	authHandler := handlers.NewAuthHandler(userRepo, cfg.JWTSecret)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files (public)
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Public routes (no auth)
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Get("/setup", authHandler.SetupPage)
	r.Post("/setup", authHandler.Setup)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(cfg.JWTSecret))

		// Logout
		r.Post("/logout", authHandler.Logout)

		// Serve 3D files and images from scan path
		r.Handle("/files/*", http.StripPrefix("/files/", http.FileServer(http.Dir(cfg.ScanPath))))

		// Pages
		r.Get("/", pageHandler.Home)
		r.Get("/models/{id}", pageHandler.ModelDetail)
		r.Get("/authors", pageHandler.Authors)
		r.Get("/tags", pageHandler.Tags)
		r.Get("/settings", settingsHandler.Page)

		// API - Models
		r.Get("/api/models", modelHandler.List)
		r.Put("/api/models/{id}", modelHandler.Update)
		r.Put("/api/models/{id}/path", modelHandler.UpdatePath)
		r.Post("/api/models/{id}/tags", modelHandler.AddTag)
		r.Get("/api/models/{id}/tags/search", modelHandler.SearchTags)
		r.Post("/api/models/{id}/tags/add", modelHandler.AddTagByName)
		r.Delete("/api/models/{id}/tags/{tagId}", modelHandler.RemoveTag)
		r.Put("/api/models/{id}/author", modelHandler.SetAuthor)
		r.Get("/api/models/{id}/author/search", modelHandler.SearchAuthors)
		r.Post("/api/models/{id}/author/set", modelHandler.SetAuthorByName)
		r.Delete("/api/models/{id}/images/hide", modelHandler.HideImage)
		r.Delete("/api/models/{id}", modelHandler.DeleteModel)
		r.Get("/api/models/{id}/merge-candidates", modelHandler.MergeCandidates)
		r.Post("/api/models/{id}/merge", modelHandler.Merge)
		r.Put("/api/models/{id}/toggle-hidden", modelHandler.ToggleHidden)
		r.Get("/api/models/{id}/category/search", modelHandler.SearchCategories)
		r.Put("/api/models/{id}/category", modelHandler.SetCategory)

		// API - Tags
		r.Post("/api/tags", tagHandler.Create)
		r.Put("/api/tags/{id}", tagHandler.Update)
		r.Delete("/api/tags/{id}", tagHandler.Delete)

		// API - Authors
		r.Post("/api/authors", authorHandler.Create)
		r.Put("/api/authors/{id}", authorHandler.Update)
		r.Delete("/api/authors/{id}", authorHandler.Delete)

		// API - Scanner
		r.Post("/api/scan", scanHandler.StartScan)
		r.Get("/api/scan/status", scanHandler.Status)

		// API - Settings
		r.Post("/api/settings/scan", settingsHandler.ForceScan)
		r.Put("/api/settings", settingsHandler.SaveSettings)
		r.Put("/api/settings/scanner-depth", settingsHandler.SaveScannerDepth)
		r.Put("/api/settings/ignored-folders", settingsHandler.SaveIgnoredFolders)
		r.Post("/api/settings/ignored-folders/add", settingsHandler.AddIgnoredFolder)
		r.Put("/api/settings/excluded-folders", settingsHandler.SaveExcludedFolders)
		r.Delete("/api/settings/excluded-paths", settingsHandler.RemoveExcludedPath)

		// API - Settings: Users
		r.Post("/api/settings/users", settingsHandler.CreateUser)
		r.Delete("/api/settings/users/{id}", settingsHandler.DeleteUser)

		// API - Categories
		r.Get("/api/categories/{id}/children", categoryHandler.GetChildren)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		<-ctx.Done()
		log.Println("Shutting down server...")
		srv.Shutdown(context.Background())
	}()

	log.Printf("Server starting on http://localhost%s", addr)
	log.Printf("Scan path: %s", cfg.ScanPath)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
