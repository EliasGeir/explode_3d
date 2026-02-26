# Favorites Feature

## Overview

Users can mark 3D models as favorites. Favorites are private — each user has their own list. The feature includes:

- A star button on every model card (home grid)
- A star button on the model detail page
- A profile page (`/profile`) with personal data editing and favorites grouped by category

## Database

Table `user_favorites` (added in `internal/database/migrations.go`):

| Column     | Type        | Notes                        |
|------------|-------------|------------------------------|
| user_id    | INTEGER     | FK → users(id) ON DELETE CASCADE |
| model_id   | INTEGER     | FK → models(id) ON DELETE CASCADE |
| created_at | TIMESTAMPTZ | default NOW()                |

Primary key: `(user_id, model_id)`.

## Architecture

### Repository

`internal/repository/favorites.go` — `FavoritesRepository`:

| Method              | Description                                          |
|---------------------|------------------------------------------------------|
| `Add(userID, modelID)` | Insert favorite (idempotent, ON CONFLICT DO NOTHING) |
| `Remove(userID, modelID)` | Delete favorite                                  |
| `IsFavorite(userID, modelID)` | Check if model is favorited                |
| `GetFavoriteIDs(userID)` | Return slice of favorited model IDs               |
| `GetFavoritesGrouped(userID)` | Return favorites with category info, sorted by category then name |

### Handlers

**`internal/handlers/favorites.go`** — `FavoritesHandler`:
- `POST /api/models/{id}/favorite` → adds favorite, returns `StarButton` HTML (outerHTML swap)
- `DELETE /api/models/{id}/favorite` → removes favorite, returns `StarButton` HTML (outerHTML swap)

**`internal/handlers/profile.go`** — `ProfileHandler`:
- `GET /profile` → renders full profile page
- `PUT /api/profile` → updates username/email, returns flash message HTML
- `PUT /api/profile/password` → verifies old password, updates to new one, returns flash message HTML
- `GET /api/profile/favorites` → returns `FavoritesGrid` HTML fragment (used by the remove button on profile page)

### Templates

`templates/profile.templ`:
- `ProfilePage(data ProfileData)` — full page
- `profileContent(data ProfileData)` — two sections: personal data + favorites
- `FavoritesGrid(favorites)` — grouped grid of favorite tiles
- `FavoriteTile(f FavoriteModel)` — single tile with thumbnail and hover-remove button
- `StarButton(modelID, favorited bool)` — toggleable star button used across home and detail pages

`templates/home.templ`:
- `HomeData.UserFavoriteIDs` — IDs of user's favorites, loaded by `PageHandler.Home`
- `ModelCard` — now includes `StarButton` in the top-left corner of the thumbnail

`templates/model_detail.templ`:
- `ModelDetailData.Favorited` — bool, loaded by `PageHandler.ModelDetail`
- Separate `#favorite-section` div above `#model-info` with the star button

`templates/layout.templ`:
- Username in navbar is now a link to `/profile`

## HTMX Patterns

### Star button (home grid / model detail)
```
hx-post/delete="/api/models/{id}/favorite"
hx-target="this"
hx-swap="outerHTML"
```
The handler returns the updated `StarButton` component to replace itself.

### Remove from profile
```
hx-delete="/api/models/{id}/favorite"
hx-on:htmx:after-request="htmx.ajax('GET','/api/profile/favorites',{target:'#profile-favorites',swap:'innerHTML'})"
```
The delete removes the favorite, then a secondary GET reloads the full favorites grid.

## User Repository additions

`internal/repository/users.go`:
- `UpdateProfile(id, username, email)` — updates user profile data
- `VerifyPassword(id, password)` — checks current password with bcrypt

## Routes

All routes require authentication (`RequireAuth` middleware):

```
GET  /profile                   → profileHandler.Page
PUT  /api/profile               → profileHandler.UpdateProfile
PUT  /api/profile/password      → profileHandler.ChangePassword
GET  /api/profile/favorites     → profileHandler.FavoritesList
POST /api/models/{id}/favorite  → favoritesHandler.Add
DELETE /api/models/{id}/favorite → favoritesHandler.Remove
```
