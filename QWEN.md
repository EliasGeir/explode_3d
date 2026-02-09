# 3D Models Categorization Application

## Project Overview

This is a Go-based web application designed to categorize, tag, and manage 3D models stored in a local directory. The application provides a web interface to browse, tag, and organize 3D models (STL, OBJ, 3MF, etc.) with support for thumbnails and author attribution.

### Key Features
- Automatic scanning of 3D model directories
- Web-based UI for browsing and managing models
- Tagging system for categorization
- Author attribution for models
- Thumbnail support for models
- SQLite database for storing metadata
- HTMX-powered dynamic UI
- TailwindCSS for styling

### Technology Stack
- **Backend**: Go (Golang)
- **Template Engine**: `templ` for type-safe HTML templates
- **Web Framework**: `chi` router
- **Database**: SQLite with FTS5 support
- **Frontend**: HTMX, TailwindCSS
- **3D Viewer**: Custom JavaScript viewer

### Architecture
- **main.go**: Entry point with HTTP server setup
- **internal/config**: Configuration loading from environment variables
- **internal/database**: SQLite database connection and initialization
- **internal/handlers**: HTTP route handlers
- **internal/models**: Data structures for models, tags, authors
- **internal/repository**: Database access layer
- **internal/scanner**: Directory scanning and model detection logic
- **templates**: UI components using templ
- **static**: CSS and JavaScript assets

## Building and Running

### Prerequisites
- Go 1.25.6 or later
- CGO enabled (required for SQLite driver)

### Build Commands
```bash
# Generate templ files and build the application
make build

# Build and run the application
make run

# Generate templ files (before building)
make generate

# Clean generated files
make clean
```

### Manual Build
```bash
# Generate templ files
templ generate ./...

# Build with CGO enabled and FTS5 support
CGO_ENABLED=1 go build -tags fts5 -o 3dmodels .

# Run the application
./3dmodels
```

### Configuration
The application uses environment variables defined in `.env`:
- `SCAN_PATH`: Directory containing 3D models (default: `/Volumes/Modelli3D`)
- `PORT`: Web server port (default: `8080`)
- `DB_PATH`: SQLite database path (default: `./data/models.db`)

## Development Conventions

### Code Structure
- Handlers follow RESTful patterns with separate routes for API and pages
- Repository pattern for database operations
- Configuration loaded via environment variables
- Scanner recursively detects 3D model directories with intelligent grouping

### Template System
- Uses `templ` for type-safe HTML templates
- Templates are generated to corresponding `_templ.go` files
- Layout component provides consistent page structure

### Database Schema
- SQLite database with tables for models, tags, authors, and relationships
- FTS5 support for full-text search capabilities
- Model files are stored separately with references to parent models

### Scanner Logic
The scanner intelligently identifies 3D model collections by:
- Detecting directories with direct 3D files as individual models
- Grouping subdirectories with common names (STL, OBJ, bases, parts, etc.) as part of the same model
- Identifying render/image directories for thumbnails
- Tracking model changes and removing stale entries