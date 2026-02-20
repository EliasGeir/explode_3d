# 3D Models Categorization

A self-hosted web application for cataloging, browsing, and managing large collections of 3D model files. It automatically scans your filesystem to discover models, organizes them into categories, and provides a rich UI with full-text search, tagging, an integrated 3D viewer, and more.

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-4169E1?logo=postgresql&logoColor=white)
![HTMX](https://img.shields.io/badge/HTMX-1.9-3366CC)
![Three.js](https://img.shields.io/badge/Three.js-WebGL-000000?logo=threedotjs&logoColor=white)

## Features

- **Automatic filesystem scanning** — recursively discovers 3D model directories with configurable detection rules
- **Full-text search** — PostgreSQL TSVECTOR with GIN index for fast prefix-matching queries
- **Hierarchical categories** — auto-generated from directory structure with recursive filtering
- **Tagging system** — create, assign, and filter by colored tags (multi-tag intersection)
- **Author tracking** — associate models with creators/sources
- **Built-in 3D viewer** — renders STL (binary & ASCII) and OBJ files in the browser using Three.js
- **Image gallery** — discovers and displays reference images, renders, and photos
- **Model merging** — transactional merge of duplicate models (files, tags, metadata)
- **Scheduled scans** — automatic daily scans at a configurable hour
- **Dark-themed UI** — responsive design with Tailwind CSS and HTMX-driven partial updates
- **No CGO required** — uses the pure-Go `pgx` driver for PostgreSQL

## Screenshots

*(Add screenshots of the home grid, model detail page, and 3D viewer here.)*

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go |
| Router | [chi](https://github.com/go-chi/chi) v5 |
| Database | PostgreSQL (via [pgx](https://github.com/jackc/pgx) v5) |
| Templates | [Templ](https://templ.guide/) |
| Frontend | HTMX + Tailwind CSS (CDN) |
| 3D Viewer | Three.js (CDN) |
| Config | [godotenv](https://github.com/joho/godotenv) |

## Prerequisites

- **Go 1.25+**
- **PostgreSQL 15+**
- **[Templ CLI](https://templ.guide/quick-start/installation)** (`go install github.com/a-h/templ/cmd/templ@latest`)
- A directory tree containing your 3D model files

## Installation

```bash
git clone https://github.com/your-username/3DModelsCategorization.git
cd 3DModelsCategorization
```

### Configuration

Create a `.env` file in the project root:

```env
SCAN_PATH=/path/to/your/3d-models    # Root directory to scan
PORT=8080                             # HTTP port (default: 8080)

DB_HOST=localhost                     # PostgreSQL host
DB_PORT=5432                          # PostgreSQL port
DB_USER=postgres                      # PostgreSQL user
DB_PASSWORD=your_password             # PostgreSQL password
DB_NAME=models3d                      # Database name (default: models3d)
DB_SSLMODE=disable                    # SSL mode (default: disable)
```

### Database Setup

Create the database — all tables, indexes, and triggers are applied automatically on startup:

```sql
CREATE DATABASE models3d;
```

### Build & Run

```bash
make build    # Generate templ files + compile Go binary
make run      # Build and start the server
```

The app will be available at `http://localhost:8080` (or your configured port).

## Build Commands

| Command | Description |
|---------|-------------|
| `make build` | Generate templ templates and compile the Go binary |
| `make run` | Build and start the application |
| `make generate` | Run `templ generate ./...` only |
| `make clean` | Remove the binary and generated `*_templ.go` files |

## Project Structure

```
.
├── main.go                          # Entry point: config, DB, router, server
├── Makefile                         # Build automation
├── .env                             # Environment configuration
├── internal/
│   ├── config/
│   │   └── config.go                # .env parsing
│   ├── models/
│   │   └── models.go                # Data structures (Model3D, Tag, Author, etc.)
│   ├── database/
│   │   ├── database.go              # PostgreSQL connection
│   │   └── migrations.go            # Auto-migrations, indexes, triggers
│   ├── repository/
│   │   ├── models.go                # Model CRUD, search, merge
│   │   ├── tags.go                  # Tag CRUD
│   │   ├── authors.go               # Author CRUD
│   │   ├── category_repository.go   # Category tree operations
│   │   └── settings.go              # Key-value settings store
│   ├── scanner/
│   │   ├── scanner.go               # Filesystem scanner
│   │   └── scheduler.go             # Automatic daily scan scheduler
│   └── handlers/
│       ├── pages.go                 # Page handlers (home, detail)
│       ├── models.go                # Model API endpoints
│       ├── tags.go                  # Tag API endpoints
│       ├── authors.go               # Author API endpoints
│       ├── scanner.go               # Scan control endpoints
│       ├── settings.go              # Settings endpoints
│       └── categories.go            # Category tree endpoints
├── templates/                       # Templ components (flat, single package)
│   ├── layout.templ                 # HTML layout wrapper
│   ├── home.templ                   # Home page grid and pagination
│   ├── model_detail.templ           # Model detail page (viewer, gallery, metadata)
│   ├── tags.templ                   # Tags management UI
│   ├── authors.templ                # Authors management UI
│   ├── settings.templ               # Settings page
│   ├── scanner_status.templ         # Scan progress indicator
│   ├── category_sidebar.templ       # Category navigation tree
│   ├── category_children_list.templ # Category children list
│   └── merge.templ                  # Model merge UI
└── static/
    ├── css/app.css                  # Custom styles (scrollbars, transitions)
    └── js/viewer3d.js               # Three.js STL/OBJ viewer
```

## Architecture

```
.env → config.Load()
         ↓
     database.Open()  →  auto-migrations + TSVECTOR/GIN setup
         ↓
     repositories (models, tags, authors, categories, settings)
         ↓
     scanner.New()  →  background goroutine, mutex-protected status
         ↓
     handlers  →  templ components  →  HTMX in browser
```

The application follows a layered architecture:

1. **Config** — loads environment variables from `.env`
2. **Database** — connects to PostgreSQL and runs auto-migrations on startup (tables, indexes, triggers)
3. **Repositories** — data access layer with typed Go methods for each entity
4. **Scanner** — background filesystem scanner that discovers models and syncs them to the database
5. **Handlers** — HTTP handlers that receive requests and render templ components
6. **Templates** — type-safe HTML components rendered server-side and swapped into the page via HTMX

## How the Scanner Works

The scanner recursively walks the configured `SCAN_PATH` to discover 3D model directories.

### Supported file formats

`.stl`, `.obj`, `.lys`, `.3mf`, `.3ds`

### Detection rules

1. **Direct model** — a directory that contains 3D files directly is treated as a model
2. **Parent model** — a directory whose subdirectories have "ignored" names (e.g., `STL`, `Base`, `LYS`, `25mm`) and contain 3D files — the parent is treated as the model
3. **Deep search** — subdirectories are searched recursively up to 5 levels deep for 3D files
4. **Category folder** — directories above `min_depth` that contain no 3D files are treated as categories

### Configurable settings

| Setting | Description | Default |
|---------|-------------|---------|
| `ignored_folder_names` | Subdirectory names to absorb into parent model (comma-separated) | `stl,obj,3mf,lys,base,parts,...` |
| `scanner_min_depth` | Minimum depth before model detection starts | `2` |
| `excluded_folders` | Directories to skip entirely during scan | *(empty)* |

A regex pattern `\d{2,3}mm` is always appended to the ignored names list to match size-variant folders like `25mm`, `32mm`, etc.

### Thumbnail detection

The scanner looks for preview images in this priority order:

1. Direct images in the model directory
2. Images in render subdirectories (`renders/`, `imgs/`, `images/`, `pictures/`, `photos/`)
3. Recursive search up to 3 levels deep

Supported image formats: PNG, JPG, JPEG, GIF, WEBP, BMP.

### Scheduled scans

When enabled, the scheduler runs daily at a configurable hour (default: 3 AM). Stale models that were not seen during a scan are automatically removed.

## Database Schema

Tables are auto-created on startup. Key tables:

| Table | Purpose |
|-------|---------|
| `models` | 3D model entries with name, path, metadata, and `search_vector` (TSVECTOR) |
| `model_files` | Individual files within each model (path, name, extension, size) |
| `tags` | Named, colored labels |
| `model_tags` | Many-to-many relation between models and tags |
| `authors` | Model creators/sources with optional URL |
| `categories` | Hierarchical directory categories with parent/depth tracking |
| `settings` | Key-value configuration store |
| `model_groups` | Named groups of related models |

Full-text search is powered by a `search_vector` TSVECTOR column on `models`, maintained by a `BEFORE INSERT OR UPDATE` trigger and indexed with a GIN index.

## API Endpoints

### Pages

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Home page — model grid with search, filters, pagination |
| GET | `/models/{id}` | Model detail — 3D viewer, images, metadata editor |
| GET | `/authors` | Authors management page |
| GET | `/tags` | Tags management page |
| GET | `/settings` | Settings and scanner configuration |

### Models API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/models` | List models (supports query, tags, author, category, pagination) |
| PUT | `/api/models/{id}` | Update model name and notes |
| PUT | `/api/models/{id}/path` | Change model path (auto-merges on conflict) |
| DELETE | `/api/models/{id}` | Delete model from database and filesystem |
| PUT | `/api/models/{id}/toggle-hidden` | Toggle model visibility |
| PUT | `/api/models/{id}/category` | Assign model to a category |
| POST | `/api/models/{id}/tags` | Add a tag by ID |
| POST | `/api/models/{id}/tags/add` | Add a tag by name (creates if missing) |
| DELETE | `/api/models/{id}/tags/{tagId}` | Remove a tag |
| GET | `/api/models/{id}/tags/search` | Search tags (typeahead) |
| PUT | `/api/models/{id}/author` | Set author by ID |
| POST | `/api/models/{id}/author/set` | Set author by name (creates if missing) |
| GET | `/api/models/{id}/author/search` | Search authors (typeahead) |
| DELETE | `/api/models/{id}/images/hide` | Hide an image |
| GET | `/api/models/{id}/merge-candidates` | Find similar models for merging |
| POST | `/api/models/{id}/merge` | Merge two models (transactional) |

### Tags, Authors, Scanner, Settings, Categories

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/tags` | Create tag |
| PUT | `/api/tags/{id}` | Update tag |
| DELETE | `/api/tags/{id}` | Delete tag |
| POST | `/api/authors` | Create author |
| PUT | `/api/authors/{id}` | Update author |
| DELETE | `/api/authors/{id}` | Delete author |
| POST | `/api/scan` | Start a scan |
| GET | `/api/scan/status` | Get current scan status |
| PUT | `/api/settings` | Save auto-scan settings |
| PUT | `/api/settings/scanner-depth` | Set minimum scanner depth |
| PUT | `/api/settings/ignored-folders` | Set ignored folder names |
| POST | `/api/settings/ignored-folders/add` | Add an ignored folder |
| PUT | `/api/settings/excluded-folders` | Set excluded folders |
| GET | `/api/categories/{id}/children` | Get child categories |

## 3D Viewer

The built-in viewer (`static/js/viewer3d.js`) is a custom Three.js implementation that renders STL and OBJ files directly in the browser:

- Parses both **binary and ASCII STL** files
- Handles **OBJ face triangulation** for non-triangle polygons
- Auto-computes vertex normals
- Orbit controls (drag to rotate, scroll to zoom)
- Automatic camera positioning based on model bounding box
- Grid helper and ambient + directional lighting
- Tab-based navigation when a model contains multiple files

No external Three.js loader libraries are required.

## Frontend

The UI is built with:

- **Templ** — type-safe, compiled HTML templates (server-side rendering)
- **HTMX** — most interactions use `hx-get`, `hx-post`, `hx-put`, `hx-delete` to swap HTML fragments without full page reloads
- **Tailwind CSS** (CDN) — dark theme with indigo accents, no build step
- **Responsive grid** — model cards adapt from 1 to 5 columns based on viewport width

All form submissions use `application/x-www-form-urlencoded` (via `URLSearchParams` in JavaScript) to ensure compatibility with Go's `r.ParseForm()`.

## License

*(Add your license here.)*