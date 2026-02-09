# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build        # templ generate + CGO_ENABLED=1 go build -tags fts5
make run          # build + run binary
make generate     # templ generate ./... only
make clean        # remove binary and generated *_templ.go files
```

**Critical:** FTS5 requires the build tag `fts5` and `CGO_ENABLED=1`. The go-sqlite3 driver uses `//go:build sqlite_fts5 || fts5` to conditionally enable it. Do NOT use `CGO_CFLAGS`.

## Architecture

Go web app: **chi router + SQLite (mattn/go-sqlite3 with FTS5) + templ + HTMX + Tailwind CDN + Three.js**

Module name: `3dmodels`

### Layer flow

```
.env → config.Load()
         ↓
     database.Open()  →  auto-migrations + FTS5 setup
         ↓
     repositories (models, tags, authors, settings)
         ↓
     scanner.New()  →  background goroutine, mutex-protected status
         ↓
     handlers  →  templ components  →  HTMX in browser
```

### Key conventions

- **Templates:** All `.templ` files must be in the flat `templates/` directory (no subdirectories) — Go requires one package per directory.
- **Templ generates** `*_templ.go` files alongside `.templ` sources. Always run `templ generate ./...` (or `make generate`) after editing `.templ` files.
- **HTMX pattern:** Handlers return HTML fragments rendered via templ. Most API endpoints return partial HTML for `hx-target` swap, not JSON.
- **JS fetch to Go handlers:** Use `URLSearchParams` (not `FormData`) when sending POST/DELETE via `fetch()`. Go's `r.ParseForm()` only reads `application/x-www-form-urlencoded` bodies; if `ParseForm` runs before `FormValue`, multipart bodies from `FormData` won't be parsed.

### Scanner

`internal/scanner/scanner.go` — recursively walks `SCAN_PATH` to discover 3D model folders.

Detection logic:
1. A directory with **direct** 3D files (`.stl`, `.obj`, `.lys`, `.3mf`, `.3ds`) → treated as a model
2. A directory whose subdirectories have "ignored" names (STL, Base, LYS, 25mm, etc.) containing 3D files → parent treated as the model
3. Otherwise → recurse into children (it's a category folder)

Ignored folder names are stored in the `settings` table (`ignored_folder_names` key) as a comma-separated list. A regex is built dynamically at scan start; `\d{2,3}mm` is always appended.

### Database

SQLite with auto-migrations on startup (`internal/database/migrations.go`). Tables: `models`, `model_files`, `tags`, `model_tags`, `authors`, `settings`, `model_groups`. FTS5 virtual table `models_fts` with insert/update/delete triggers on `models.name`.

### Frontend

- Tailwind CSS via CDN (no build step)
- HTMX for dynamic partial updates
- `static/js/viewer3d.js` — custom Three.js STL/OBJ viewer with orbit controls, no external loader libraries
- 3D files and images served from scan path at `/files/*`

## Configuration

`.env` file with: `SCAN_PATH` (root folder to scan), `PORT` (default 8080), `DB_PATH` (default `./data/models.db`).
