# 3D Models Categorization — Full Documentation

*[Leggi in italiano](README.it.md)* | *[Back to README](../../README.md)*

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Installation](#installation)
4. [Configuration](#configuration)
5. [How the Scanner Works](#how-the-scanner-works)
6. [Database Schema](#database-schema)
7. [Authentication & Roles](#authentication--roles)
8. [Internationalization (i18n)](#internationalization-i18n)
9. [Frontend & UI](#frontend--ui)
10. [3D Viewer](#3d-viewer)
11. [API Endpoints](#api-endpoints)
12. [User Guide](#user-guide)

---

## Overview

3D Models Categorization is a self-hosted Go web application designed to manage large collections of 3D printing files (STL, OBJ, LYS, 3MF, 3DS). It scans a filesystem directory tree, organizes models into hierarchical categories, and provides a feature-rich web UI for browsing, searching, tagging, and previewing.

Key design goals:
- **Zero external frontend build tools** — Tailwind CSS and HTMX are loaded via CDN
- **No CGO** — the pure-Go `pgx` PostgreSQL driver means easy cross-compilation
- **Server-side rendering** — all HTML is rendered by templ components; HTMX swaps in partial updates
- **Minimal configuration** — one `.env` file and a PostgreSQL database

## Architecture

```
.env → config.Load()
         ↓
     database.Open()  →  auto-migrations + TSVECTOR/GIN setup
         ↓
     repositories (models, tags, authors, categories, settings, users, feedback, favorites)
         ↓
     scanner.New()  →  background goroutine, mutex-protected status
         ↓
     i18n.Load()  →  embedded JSON locale files (IT/EN)
         ↓
     handlers  →  templ components  →  HTMX in browser
```

### Layer breakdown

| Layer | Package | Responsibility |
|-------|---------|---------------|
| Config | `internal/config` | Loads `.env` variables |
| Database | `internal/database` | PostgreSQL connection pool, auto-migrations |
| Models | `internal/models` | Go structs for all entities |
| Repository | `internal/repository` | Data access (one file per entity) |
| Scanner | `internal/scanner` | Filesystem discovery + background scheduler |
| i18n | `internal/i18n` | Translation loading, middleware, `T()` function |
| Middleware | `internal/middleware` | JWT authentication, role-based access |
| Handlers | `internal/handlers` | HTTP endpoints (pages + API) |
| Templates | `templates/` | Templ components (flat single-package directory) |

### Key conventions

- **Templates** must all be in the flat `templates/` directory — Go requires one package per directory
- **HTMX pattern** — handlers return HTML fragments for `hx-target` swap, not JSON
- **Form encoding** — use `URLSearchParams` (not `FormData`) in JS; Go's `r.ParseForm()` only reads `application/x-www-form-urlencoded`
- **SQL placeholders** — PostgreSQL uses `$1, $2, $3...`, not `?`

## Installation

### Prerequisites

- Go 1.25+
- PostgreSQL 15+
- Templ CLI: `go install github.com/a-h/templ/cmd/templ@latest`

### Steps

```bash
# 1. Clone
git clone https://github.com/your-username/3DModelsCategorization.git
cd 3DModelsCategorization

# 2. Create .env (see Configuration below)

# 3. Create PostgreSQL database
psql -U postgres -c "CREATE DATABASE models3d;"

# 4. Build and run
make build
make run
```

On first launch, navigate to `http://localhost:8080` — you will be redirected to the setup page to create the first admin account.

## Configuration

All configuration is via a `.env` file in the project root:

| Variable | Description | Default |
|----------|-------------|---------|
| `SCAN_PATH` | Root directory to scan for 3D models | *(required)* |
| `PORT` | HTTP port | `8080` |
| `DB_HOST` | PostgreSQL host | `localhost` |
| `DB_PORT` | PostgreSQL port | `5432` |
| `DB_USER` | PostgreSQL user | `postgres` |
| `DB_PASSWORD` | PostgreSQL password | *(required)* |
| `DB_NAME` | Database name | `models3d` |
| `DB_SSLMODE` | SSL mode | `disable` |

## How the Scanner Works

The scanner (`internal/scanner/scanner.go`) recursively walks `SCAN_PATH` to discover 3D model directories.

### Supported file formats

`.stl`, `.obj`, `.lys`, `.3mf`, `.3ds`

### Detection rules

1. **Direct model** — a directory containing 3D files directly is treated as a model
2. **Parent model** — a directory whose subdirectories have "ignored" names (e.g., `STL`, `Base`, `25mm`) and contain 3D files — the parent is the model
3. **Deep search** — subdirectories are searched recursively (up to 5 levels) for 3D files
4. **Category folder** — directories above `scanner_min_depth` with no direct 3D files become categories

### Configurable settings (via Settings UI)

| Setting | Description | Default |
|---------|-------------|---------|
| `ignored_folder_names` | Subdirectory names to absorb into parent model | `stl,obj,3mf,lys,base,parts,...` |
| `scanner_min_depth` | Minimum folder depth before model detection starts | `2` |
| `excluded_folders` | Directories to skip entirely during scan | *(empty)* |

The regex `\d{2,3}mm` is always appended to match size-variant folders (25mm, 32mm, etc.).

### Thumbnail detection

Priority: direct images → images in render subdirectories (`renders/`, `imgs/`, `images/`, etc.) → recursive search (3 levels). Supported: PNG, JPG, JPEG, GIF, WEBP, BMP.

### Scheduled scans

When enabled, runs daily at a configurable hour (default: 3 AM). Stale models not seen during a scan are automatically removed.

## Database Schema

Tables are auto-created on startup via `internal/database/migrations.go`.

| Table | Purpose |
|-------|---------|
| `models` | 3D model entries with name, path, metadata, `search_vector` (TSVECTOR) |
| `model_files` | Individual files within each model |
| `tags` | Named, colored labels |
| `model_tags` | Many-to-many: models ↔ tags |
| `authors` | Model creators/sources with optional URL |
| `categories` | Hierarchical directory categories (parent/depth tracking) |
| `settings` | Key-value configuration store |
| `model_groups` | Named groups of related models |
| `users` | User accounts (username, email, bcrypt hash) |
| `roles` | Roles (ROLE_ADMIN, ROLE_USER) |
| `user_roles` | Many-to-many: users ↔ roles |
| `user_favorites` | Many-to-many: users ↔ favorite models |
| `feedback` | User feedback submissions with status tracking |
| `feedback_categories` | Feedback categories (icon, color, sort order) |

Full-text search uses a `search_vector TSVECTOR` column on `models`, maintained by a `BEFORE INSERT OR UPDATE` trigger, indexed with GIN.

## Authentication & Roles

- **JWT-based** — tokens stored in an HTTP cookie (`token`)
- On first launch, `/setup` creates the initial admin account
- Two roles: `ROLE_ADMIN` (full access + settings) and `ROLE_USER` (browse + edit models)
- Middleware: `RequireAuth` (all protected routes) and `RequireRole("ROLE_ADMIN")` (admin routes)
- Admin can create users and assign/remove roles from the Settings > Users tab

## Internationalization (i18n)

The UI supports **Italian** (default) and **English**, switchable at any time.

### How it works

- Package `internal/i18n` loads embedded JSON locale files (`locales/it.json`, `locales/en.json`) via `go:embed`
- Middleware reads the language from: 1) `lang` cookie → 2) `Accept-Language` header → 3) default (IT)
- Templates use `i18n.T(ctx, "key")` or `i18n.T(ctx, "key", args...)` for translated strings
- Language switch: click **IT** or **EN** in the navbar → `GET /set-lang?lang=xx` → sets cookie → redirects back

### Translation keys

Keys use dot notation organized by section: `nav.*`, `home.*`, `model.*`, `merge.*`, `tags.*`, `authors.*`, `auth.*`, `profile.*`, `settings.*`, `feedback.*`, `scanner.*`, `sidebar.*`, `common.*`.

Database content (model names, tags, authors, categories) is **not translated**.

## Frontend & UI

| Technology | Role |
|-----------|------|
| **Templ** | Type-safe compiled HTML templates (server-side rendering) |
| **HTMX** | Partial page updates via `hx-get`, `hx-post`, `hx-put`, `hx-delete` |
| **Tailwind CSS** (CDN) | Dark theme with indigo accents, no build step |
| **Three.js** (CDN) | 3D model rendering in the browser |

### Pages

| Page | Description |
|------|-------------|
| **Home** (`/`) | Model grid with category tabs, search bar, pagination, favorite stars |
| **Model Detail** (`/models/{id}`) | 3D viewer, image gallery, metadata editor (name, notes, author, category, tags, visibility), file list, merge/delete |
| **Authors** (`/authors`) | Author list with model count, add/delete |
| **Tags** (`/tags`) | Tag list with color, model count, add/delete |
| **Profile** (`/profile`) | Personal data, password change, favorites grouped by category |
| **Settings** (`/settings`) | Scanner config, paths, user management (admin only) |
| **Feedback** (`/feedback`) | Feedback list with status management, category management (admin only) |
| **Login** (`/login`) | Authentication form with language switch |

## 3D Viewer

The built-in viewer (`static/js/viewer3d.js`) is a custom Three.js implementation:

- Parses both **binary and ASCII STL** files
- Handles **OBJ face triangulation** for non-triangle polygons
- Auto-computes vertex normals
- Orbit controls (drag to rotate, scroll to zoom)
- Automatic camera positioning based on bounding box
- Grid helper and ambient + directional lighting
- Tab-based file navigation when a model contains multiple viewable files
- Click on a file in the file list to jump to that file in the viewer

No external Three.js loader libraries are required.

## API Endpoints

### Public routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/login` | Login page |
| POST | `/login` | Authenticate |
| GET | `/setup` | First-time setup page |
| POST | `/setup` | Create first admin |
| GET | `/set-lang` | Switch language (cookie) |

### Protected routes (require authentication)

#### Pages

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Home page |
| GET | `/models/{id}` | Model detail |
| GET | `/authors` | Authors page |
| GET | `/tags` | Tags page |
| GET | `/profile` | User profile |

#### Models API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/models` | List models (query, tags, author, category, pagination) |
| PUT | `/api/models/{id}` | Update name and notes |
| PUT | `/api/models/{id}/path` | Change path (auto-merges on conflict) |
| DELETE | `/api/models/{id}` | Delete model + files from disk |
| PUT | `/api/models/{id}/toggle-hidden` | Toggle visibility |
| PUT | `/api/models/{id}/category` | Assign category |
| POST | `/api/models/{id}/tags` | Add tag by ID |
| POST | `/api/models/{id}/tags/add` | Add tag by name (creates if missing) |
| DELETE | `/api/models/{id}/tags/{tagId}` | Remove tag |
| GET | `/api/models/{id}/tags/search` | Typeahead search tags |
| PUT | `/api/models/{id}/author` | Set author by ID |
| POST | `/api/models/{id}/author/set` | Set author by name |
| GET | `/api/models/{id}/author/search` | Typeahead search authors |
| GET | `/api/models/{id}/category/search` | Typeahead search categories |
| DELETE | `/api/models/{id}/images/hide` | Hide an image |
| GET | `/api/models/{id}/merge-candidates` | Find merge candidates |
| POST | `/api/models/{id}/merge` | Merge models |
| POST | `/api/models/{id}/favorite` | Add to favorites |
| DELETE | `/api/models/{id}/favorite` | Remove from favorites |

#### Tags, Authors, Scanner, Settings

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/tags` | Create tag |
| PUT | `/api/tags/{id}` | Update tag |
| DELETE | `/api/tags/{id}` | Delete tag |
| POST | `/api/authors` | Create author |
| PUT | `/api/authors/{id}` | Update author |
| DELETE | `/api/authors/{id}` | Delete author |
| POST | `/api/scan` | Start scan |
| GET | `/api/scan/status` | Scan status |
| PUT | `/api/profile` | Update profile |
| PUT | `/api/profile/password` | Change password |
| GET | `/api/profile/favorites` | Get favorites list |

#### Admin-only routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/settings` | Settings page |
| PUT | `/api/settings` | Save auto-scan settings |
| POST | `/api/settings/scan` | Force scan |
| PUT | `/api/settings/scanner-depth` | Set min depth |
| PUT | `/api/settings/ignored-folders` | Set ignored folders |
| POST | `/api/settings/ignored-folders/add` | Add ignored folder |
| PUT | `/api/settings/excluded-folders` | Set excluded folders |
| DELETE | `/api/settings/excluded-paths` | Remove excluded path |
| POST | `/api/settings/users` | Create user |
| DELETE | `/api/settings/users/{id}` | Delete user |
| POST | `/api/settings/users/{id}/roles` | Assign role |
| DELETE | `/api/settings/users/{id}/roles/{roleId}` | Remove role |
| GET | `/feedback` | Feedback admin page |
| GET | `/api/feedback` | List feedback |
| POST | `/api/feedback` | Submit feedback |
| GET | `/api/feedback/modal` | Feedback form modal |
| PUT | `/api/feedback/{id}/status` | Update status |
| DELETE | `/api/feedback/{id}` | Delete feedback |
| GET/POST/PUT/DELETE | `/api/feedback/categories/*` | Manage categories |

## User Guide

### First launch

1. Start the application and open `http://localhost:8080`
2. You'll be redirected to the **Setup** page — create the admin account
3. Log in with the credentials you just created
4. Go to **Settings** > **Scanner** and click **Force Scan Now** to index your models

### Browsing models

- Use the **category tabs** at the top to filter by top-level category
- The **sidebar** shows sub-categories when a category is selected
- Use the **search bar** to find models by name (full-text search)
- Click the **star** on any model card to add it to your favorites

### Managing a model

On the model detail page you can:
- View the 3D model in the **integrated viewer** (navigate between files using the tabs)
- Browse the **image gallery** and open images in a lightbox
- Edit the **name** and **notes**
- Assign an **author** (typeahead search, creates if not found)
- Assign a **category** (typeahead search)
- Add/remove **tags** (typeahead search, creates if not found)
- Toggle **visibility** (hidden models appear dimmed in the grid)
- **Merge** with another model (moves all files and tags)
- **Delete** the model (removes files from disk permanently)

### Settings (Admin)

The Settings page has three tabs:

- **Scanner** — view last scan time, force scan, enable scheduled daily scan, set minimum depth
- **Paths** — configure ignored folder names, excluded folders, view/manage excluded paths
- **Users** — list users, assign/remove roles, create new users

### Language

Click **IT** or **EN** in the navbar to switch language. The preference is saved in a cookie and persists across sessions.
