# i18n Implementation — Changelog

Date: 2026-02-27

## Summary

Added full frontend internationalization (i18n) supporting Italian (default) and English, with runtime language switching via the navbar.

## What was done

### New files created

| File | Purpose |
|------|---------|
| `internal/i18n/i18n.go` | Core i18n package: `Load()`, `T(ctx, key, args...)`, `WithLocale()`/`GetLocale()` context helpers. Uses `go:embed` to load JSON locale files. Fallback chain: current lang → IT → EN → return key as-is. |
| `internal/i18n/middleware.go` | Chi middleware that detects language from: 1) `lang` cookie, 2) `Accept-Language` header, 3) default IT. Sets locale in request context. |
| `internal/i18n/locales/it.json` | ~130 Italian translation keys in dot-notation (e.g., `nav.home`, `model.save`, `settings.users_title`). |
| `internal/i18n/locales/en.json` | ~130 English translation keys, same structure as `it.json`. |
| `internal/handlers/lang.go` | `GET /set-lang?lang=it\|en` endpoint. Sets `lang` cookie (1 year, HttpOnly, SameSite=Lax) and redirects to `Referer`. |

### Modified files

| File | Changes |
|------|---------|
| `main.go` | Added `i18n.Load()` at startup, `i18n.Middleware` in middleware chain, `/set-lang` public route. |
| `templates/layout.templ` | Dynamic `<html lang>`, language switch (IT \| EN) in navbar, all nav strings via `i18n.T()`. |
| `templates/login.templ` | Language switch on login/setup pages, all form strings translated. |
| `templates/home.templ` | Translated: page title, model count, "All Models" tab, pagination, empty states. |
| `templates/model_detail.templ` | ~25 strings: metadata labels, placeholders, confirmation messages, section headers. |
| `templates/merge.templ` | Script blocks modified to receive translated strings as parameters (can't use `ctx` in JS). Delete warning split into prefix/suffix around HTML tags. |
| `templates/tags.templ` | Title, form labels, delete confirm, empty state. |
| `templates/authors.templ` | Title, form labels, delete confirm, empty state. |
| `templates/profile.templ` | Profile sections, password change, favorites. |
| `templates/settings.templ` | All three tabs (Scanner, Paths, Users), many `hx-confirm` messages with interpolated args. |
| `templates/feedback.templ` | Feedback modal, admin page, status badges, category management. |
| `templates/scanner_status.templ` | Processed/removed count messages. Removed unused `fmt` import. |
| `templates/category_sidebar.templ` | Sub-categories title and empty state. |

### Translation key structure

Keys use dot-notation organized by UI section:

```
nav.*        — Navbar links, search placeholder, logout
home.*       — Home page title, model count, pagination
model.*      — Model detail labels, metadata, sections
merge.*      — Merge/delete dialog strings
tags.*       — Tags page
authors.*    — Authors page
auth.*       — Login/setup form
profile.*    — Profile page
settings.*   — Settings tabs (scanner, paths, users)
feedback.*   — Feedback modal and admin page
scanner.*    — Scanner status messages
sidebar.*    — Category sidebar
common.*     — Shared strings (save, cancel, delete, etc.)
lang.*       — Language names
```

## Technical decisions

1. **No external dependencies** — only `go:embed` + stdlib. No i18n library.
2. **Context-based locale** — stored in `context.Context`, automatically available in all templ components via `ctx`.
3. **HTMX compatibility** — partial HTML responses inherit locale from context, so HTMX swaps render in the correct language without extra work.
4. **Script blocks** — templ `script` functions can't access `ctx`, so translated strings are passed as function parameters from the calling templ component.
5. **HTML in translations avoided** — instead of using `templ.Raw()`, strings containing HTML are split into prefix/suffix parts, with HTML tags placed directly in the template between them.
6. **Database content not translated** — model names, tag names, author names, and category names remain as-is.

## Build verification

```bash
PATH="$HOME/go/bin:$PATH" templ generate ./...
PATH="$HOME/go/bin:$PATH" go build -o 3dmodels .
# Compiles successfully, binary ~17MB
```
