# GEMINI.md

This file provides guidance for interacting with the code in this repository.

## Project Overview

This is a Go web application for scanning, categorizing, and viewing a local collection of 3D models. It uses a web interface to present a library of models found on the file system.

**Technologies:**

*   **Backend:** Go
*   **Web Framework/Router:** `chi`
*   **Database:** SQLite with `mattn/go-sqlite3`, using FTS5 for full-text search.
*   **Templating:** `templ` for Go-based HTML templates.
*   **Frontend:** HTMX for dynamic UI updates, Tailwind CSS (via CDN), and a custom `Three.js` viewer for rendering `.stl` and `.obj` files.
*   **Configuration:** `.env` file.

## Architecture

The application follows a layered architecture:

1.  **Configuration (`.env`)**: Loaded by `internal/config/config.go`.
2.  **Database (`internal/database`)**: Opens the SQLite database, runs migrations, and sets up FTS5 triggers.
3.  **Repositories (`internal/repository`)**: Data access layer for models, tags, authors, and settings.
4.  **Scanner (`internal/scanner`)**: A background service that recursively scans the `SCAN_PATH` to discover and catalog 3D models.
5.  **Handlers (`internal/handlers`)**: HTTP handlers that process requests, call repositories, and render `templ` components.
6.  **Frontend (`templates/`, `static/`)**: `templ` components generate HTML fragments, which are dynamically loaded and swapped by HTMX in the browser.

## Build and Run Commands

The project uses a `Makefile` for common tasks.

*   `make build`: Generates Go code from `.templ` files and builds the final executable named `3dmodels`.
    *   **Note:** This command sets `CGO_ENABLED=1` and the `fts5` build tag, which are required for SQLite FTS5 support.
*   `make run`: Builds and runs the application.
*   `make generate`: Only runs the `templ generate` command to update the `*_templ.go` files from their `.templ` sources.
*   `make clean`: Removes the compiled binary and generated `*_templ.go` files.

## Development Conventions

*   **Templates**: All `.templ` files are located in the `templates/` directory. Do not use subdirectories, as Go expects one package per directory. Run `make generate` after any changes to `.templ` files.
*   **HTMX Driven**: Most API endpoints do not return JSON. Instead, they return HTML fragments rendered by `templ`. These fragments are designed to be swapped into the page using HTMX attributes like `hx-target`.
*   **Configuration**: The application is configured via an `.env` file in the project root. Key variables include:
    *   `SCAN_PATH`: The absolute path to the root directory containing 3D models to scan.
    *   `PORT`: The port for the web server (defaults to `8080`).
    *   `DB_PATH`: The path to the SQLite database file (defaults to `./data/models.db`).
*   **Scanner Logic**: The scanner in `internal/scanner/scanner.go` identifies model directories based on file presence and a configurable list of "ignored" sub-folder names (like "STL", "parts", "25mm", etc.).

## Frontend Details

*   **Styling**: Tailwind CSS is included via a CDN link in the main layout (`templates/layout.templ`). There is no local CSS build step.
*   **3D Viewer**: A custom vanilla JavaScript 3D viewer is located at `static/js/viewer3d.js`. It uses Three.js to render STL and OBJ files and does not depend on external loader libraries.
*   **File Serving**: Static assets are served from `/static/*`. The actual 3D model files and their preview images are served directly from the `SCAN_PATH` under the `/files/*` route.
