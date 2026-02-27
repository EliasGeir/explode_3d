# 3D Models Categorization — Documentazione Completa

*[Read in English](README.en.md)* | *[Torna al README](../../README.md)*

---

## Indice

1. [Panoramica](#panoramica)
2. [Architettura](#architettura)
3. [Installazione](#installazione)
4. [Configurazione](#configurazione)
5. [Come funziona lo Scanner](#come-funziona-lo-scanner)
6. [Schema del Database](#schema-del-database)
7. [Autenticazione e Ruoli](#autenticazione-e-ruoli)
8. [Internazionalizzazione (i18n)](#internazionalizzazione-i18n)
9. [Frontend e UI](#frontend-e-ui)
10. [Viewer 3D](#viewer-3d)
11. [Endpoint API](#endpoint-api)
12. [Guida Utente](#guida-utente)

---

## Panoramica

3D Models Categorization è un'applicazione web self-hosted scritta in Go, progettata per gestire grandi collezioni di file per la stampa 3D (STL, OBJ, LYS, 3MF, 3DS). Scansiona un albero di directory del filesystem, organizza i modelli in categorie gerarchiche e fornisce un'interfaccia web ricca di funzionalità per navigare, cercare, taggare e visualizzare in anteprima.

Obiettivi di design:
- **Zero strumenti di build frontend** — Tailwind CSS e HTMX sono caricati via CDN
- **Nessun CGO** — il driver PostgreSQL pure-Go `pgx` consente una compilazione incrociata semplice
- **Rendering server-side** — tutto l'HTML è generato da componenti templ; HTMX gestisce gli aggiornamenti parziali
- **Configurazione minima** — un file `.env` e un database PostgreSQL

## Architettura

```
.env → config.Load()
         ↓
     database.Open()  →  auto-migrazioni + setup TSVECTOR/GIN
         ↓
     repository (models, tags, authors, categories, settings, users, feedback, favorites)
         ↓
     scanner.New()  →  goroutine in background, stato protetto da mutex
         ↓
     i18n.Load()  →  file JSON locale embedded (IT/EN)
         ↓
     handlers  →  componenti templ  →  HTMX nel browser
```

### Suddivisione dei livelli

| Livello | Package | Responsabilità |
|---------|---------|---------------|
| Config | `internal/config` | Carica le variabili dal `.env` |
| Database | `internal/database` | Pool di connessioni PostgreSQL, auto-migrazioni |
| Models | `internal/models` | Struct Go per tutte le entità |
| Repository | `internal/repository` | Accesso ai dati (un file per entità) |
| Scanner | `internal/scanner` | Discovery del filesystem + scheduler in background |
| i18n | `internal/i18n` | Caricamento traduzioni, middleware, funzione `T()` |
| Middleware | `internal/middleware` | Autenticazione JWT, controllo accessi basato su ruoli |
| Handlers | `internal/handlers` | Endpoint HTTP (pagine + API) |
| Templates | `templates/` | Componenti Templ (directory flat, singolo package) |

### Convenzioni principali

- I **template** devono essere tutti nella directory flat `templates/` — Go richiede un package per directory
- **Pattern HTMX** — gli handler restituiscono frammenti HTML per lo swap `hx-target`, non JSON
- **Encoding dei form** — usare `URLSearchParams` (non `FormData`) in JS; `r.ParseForm()` di Go legge solo `application/x-www-form-urlencoded`
- **Placeholder SQL** — PostgreSQL usa `$1, $2, $3...`, non `?`

## Installazione

### Prerequisiti

- Go 1.25+
- PostgreSQL 15+
- Templ CLI: `go install github.com/a-h/templ/cmd/templ@latest`

### Passaggi

```bash
# 1. Clona il repository
git clone https://github.com/your-username/3DModelsCategorization.git
cd 3DModelsCategorization

# 2. Crea il file .env (vedi Configurazione sotto)

# 3. Crea il database PostgreSQL
psql -U postgres -c "CREATE DATABASE models3d;"

# 4. Compila e avvia
make build
make run
```

Al primo avvio, naviga su `http://localhost:8080` — verrai reindirizzato alla pagina di setup per creare il primo account admin.

## Configurazione

Tutta la configurazione avviene tramite un file `.env` nella root del progetto:

| Variabile | Descrizione | Default |
|-----------|-------------|---------|
| `SCAN_PATH` | Directory radice da scansionare per i modelli 3D | *(obbligatorio)* |
| `PORT` | Porta HTTP | `8080` |
| `DB_HOST` | Host PostgreSQL | `localhost` |
| `DB_PORT` | Porta PostgreSQL | `5432` |
| `DB_USER` | Utente PostgreSQL | `postgres` |
| `DB_PASSWORD` | Password PostgreSQL | *(obbligatorio)* |
| `DB_NAME` | Nome del database | `models3d` |
| `DB_SSLMODE` | Modalità SSL | `disable` |

## Come funziona lo Scanner

Lo scanner (`internal/scanner/scanner.go`) attraversa ricorsivamente `SCAN_PATH` per scoprire le directory contenenti modelli 3D.

### Formati file supportati

`.stl`, `.obj`, `.lys`, `.3mf`, `.3ds`

### Regole di rilevamento

1. **Modello diretto** — una directory che contiene direttamente file 3D viene trattata come un modello
2. **Modello padre** — una directory le cui sottodirectory hanno nomi "ignorati" (es. `STL`, `Base`, `25mm`) e contengono file 3D — il padre è il modello
3. **Ricerca profonda** — le sottodirectory vengono cercate ricorsivamente (fino a 5 livelli) per file 3D
4. **Cartella categoria** — le directory sopra `scanner_min_depth` senza file 3D diretti diventano categorie

### Impostazioni configurabili (tramite UI Impostazioni)

| Impostazione | Descrizione | Default |
|--------------|-------------|---------|
| `ignored_folder_names` | Nomi di sottodirectory da assorbire nel modello padre | `stl,obj,3mf,lys,base,parts,...` |
| `scanner_min_depth` | Profondità minima delle cartelle prima che inizi il rilevamento dei modelli | `2` |
| `excluded_folders` | Directory da saltare completamente durante la scansione | *(vuoto)* |

La regex `\d{2,3}mm` viene sempre aggiunta per intercettare le cartelle varianti di dimensione (25mm, 32mm, ecc.).

### Rilevamento thumbnail

Priorità: immagini dirette → immagini nelle sottodirectory di render (`renders/`, `imgs/`, `images/`, ecc.) → ricerca ricorsiva (3 livelli). Formati supportati: PNG, JPG, JPEG, GIF, WEBP, BMP.

### Scansioni programmate

Quando abilitata, la scansione viene eseguita giornalmente a un'ora configurabile (default: ore 3). I modelli obsoleti non trovati durante una scansione vengono rimossi automaticamente.

## Schema del Database

Le tabelle vengono create automaticamente all'avvio tramite `internal/database/migrations.go`.

| Tabella | Scopo |
|---------|-------|
| `models` | Modelli 3D con nome, percorso, metadati, `search_vector` (TSVECTOR) |
| `model_files` | File individuali all'interno di ogni modello |
| `tags` | Etichette con nome e colore |
| `model_tags` | Many-to-many: modelli ↔ tag |
| `authors` | Creatori/fonti dei modelli con URL opzionale |
| `categories` | Categorie gerarchiche dalle directory (tracciamento padre/profondità) |
| `settings` | Archivio configurazioni chiave-valore |
| `model_groups` | Gruppi di modelli correlati |
| `users` | Account utente (username, email, hash bcrypt) |
| `roles` | Ruoli (ROLE_ADMIN, ROLE_USER) |
| `user_roles` | Many-to-many: utenti ↔ ruoli |
| `user_favorites` | Many-to-many: utenti ↔ modelli preferiti |
| `feedback` | Segnalazioni degli utenti con tracciamento dello stato |
| `feedback_categories` | Categorie feedback (icona, colore, ordine) |

La ricerca full-text usa una colonna `search_vector TSVECTOR` sulla tabella `models`, mantenuta da un trigger `BEFORE INSERT OR UPDATE`, indicizzata con GIN.

## Autenticazione e Ruoli

- **Basata su JWT** — i token sono memorizzati in un cookie HTTP (`token`)
- Al primo avvio, `/setup` crea l'account admin iniziale
- Due ruoli: `ROLE_ADMIN` (accesso completo + impostazioni) e `ROLE_USER` (navigazione + modifica modelli)
- Middleware: `RequireAuth` (tutte le route protette) e `RequireRole("ROLE_ADMIN")` (route admin)
- L'admin può creare utenti e assegnare/rimuovere ruoli dalla tab Impostazioni > Utenti

## Internazionalizzazione (i18n)

L'interfaccia supporta **Italiano** (default) e **Inglese**, selezionabili in qualsiasi momento.

### Come funziona

- Il package `internal/i18n` carica file JSON locale embedded (`locales/it.json`, `locales/en.json`) tramite `go:embed`
- Il middleware legge la lingua da: 1) cookie `lang` → 2) header `Accept-Language` → 3) default (IT)
- I template usano `i18n.T(ctx, "chiave")` o `i18n.T(ctx, "chiave", args...)` per le stringhe tradotte
- Cambio lingua: clicca **IT** o **EN** nella navbar → `GET /set-lang?lang=xx` → imposta cookie → redirect

### Chiavi di traduzione

Le chiavi usano dot-notation organizzata per sezione: `nav.*`, `home.*`, `model.*`, `merge.*`, `tags.*`, `authors.*`, `auth.*`, `profile.*`, `settings.*`, `feedback.*`, `scanner.*`, `sidebar.*`, `common.*`.

Il contenuto del database (nomi modelli, tag, autori, categorie) **non viene tradotto**.

## Frontend e UI

| Tecnologia | Ruolo |
|-----------|-------|
| **Templ** | Template HTML compilati e type-safe (rendering server-side) |
| **HTMX** | Aggiornamenti parziali della pagina via `hx-get`, `hx-post`, `hx-put`, `hx-delete` |
| **Tailwind CSS** (CDN) | Tema scuro con accenti indaco, nessun build step |
| **Three.js** (CDN) | Rendering dei modelli 3D nel browser |

### Pagine

| Pagina | Descrizione |
|--------|-------------|
| **Home** (`/`) | Griglia modelli con tab categorie, barra di ricerca, paginazione, stelle preferiti |
| **Dettaglio Modello** (`/models/{id}`) | Viewer 3D, galleria immagini, editor metadati (nome, note, autore, categoria, tag, visibilità), lista file, unisci/elimina |
| **Autori** (`/authors`) | Lista autori con conteggio modelli, aggiungi/elimina |
| **Tag** (`/tags`) | Lista tag con colore, conteggio modelli, aggiungi/elimina |
| **Profilo** (`/profile`) | Dati personali, cambio password, preferiti raggruppati per categoria |
| **Impostazioni** (`/settings`) | Configurazione scanner, percorsi, gestione utenti (solo admin) |
| **Feedback** (`/feedback`) | Lista feedback con gestione stato, gestione categorie (solo admin) |
| **Login** (`/login`) | Form di autenticazione con selettore lingua |

## Viewer 3D

Il viewer integrato (`static/js/viewer3d.js`) è un'implementazione custom di Three.js:

- Analizza file STL sia **binari che ASCII**
- Gestisce la **triangolazione delle facce OBJ** per poligoni non triangolari
- Calcolo automatico delle normali ai vertici
- Controlli orbitali (trascina per ruotare, scroll per zoom)
- Posizionamento automatico della camera basato sul bounding box
- Griglia di riferimento e illuminazione ambientale + direzionale
- Navigazione a tab quando un modello contiene più file visualizzabili
- Clicca su un file nella lista file per saltare a quel file nel viewer

Non sono richieste librerie esterne di loader per Three.js.

## Endpoint API

### Route pubbliche

| Metodo | Percorso | Descrizione |
|--------|----------|-------------|
| GET | `/login` | Pagina di login |
| POST | `/login` | Autenticazione |
| GET | `/setup` | Pagina setup iniziale |
| POST | `/setup` | Crea primo admin |
| GET | `/set-lang` | Cambio lingua (cookie) |

### Route protette (richiedono autenticazione)

#### Pagine

| Metodo | Percorso | Descrizione |
|--------|----------|-------------|
| GET | `/` | Home page |
| GET | `/models/{id}` | Dettaglio modello |
| GET | `/authors` | Pagina autori |
| GET | `/tags` | Pagina tag |
| GET | `/profile` | Profilo utente |

#### API Modelli

| Metodo | Percorso | Descrizione |
|--------|----------|-------------|
| GET | `/api/models` | Lista modelli (query, tag, autore, categoria, paginazione) |
| PUT | `/api/models/{id}` | Aggiorna nome e note |
| PUT | `/api/models/{id}/path` | Cambia percorso (auto-merge in caso di conflitto) |
| DELETE | `/api/models/{id}` | Elimina modello + file dal disco |
| PUT | `/api/models/{id}/toggle-hidden` | Attiva/disattiva visibilità |
| PUT | `/api/models/{id}/category` | Assegna categoria |
| POST | `/api/models/{id}/tags` | Aggiungi tag per ID |
| POST | `/api/models/{id}/tags/add` | Aggiungi tag per nome (crea se mancante) |
| DELETE | `/api/models/{id}/tags/{tagId}` | Rimuovi tag |
| GET | `/api/models/{id}/tags/search` | Ricerca typeahead tag |
| PUT | `/api/models/{id}/author` | Imposta autore per ID |
| POST | `/api/models/{id}/author/set` | Imposta autore per nome |
| GET | `/api/models/{id}/author/search` | Ricerca typeahead autori |
| GET | `/api/models/{id}/category/search` | Ricerca typeahead categorie |
| DELETE | `/api/models/{id}/images/hide` | Nascondi un'immagine |
| GET | `/api/models/{id}/merge-candidates` | Trova candidati per merge |
| POST | `/api/models/{id}/merge` | Unisci modelli |
| POST | `/api/models/{id}/favorite` | Aggiungi ai preferiti |
| DELETE | `/api/models/{id}/favorite` | Rimuovi dai preferiti |

#### Tag, Autori, Scanner, Impostazioni

| Metodo | Percorso | Descrizione |
|--------|----------|-------------|
| POST | `/api/tags` | Crea tag |
| PUT | `/api/tags/{id}` | Aggiorna tag |
| DELETE | `/api/tags/{id}` | Elimina tag |
| POST | `/api/authors` | Crea autore |
| PUT | `/api/authors/{id}` | Aggiorna autore |
| DELETE | `/api/authors/{id}` | Elimina autore |
| POST | `/api/scan` | Avvia scansione |
| GET | `/api/scan/status` | Stato scansione |
| PUT | `/api/profile` | Aggiorna profilo |
| PUT | `/api/profile/password` | Cambia password |
| GET | `/api/profile/favorites` | Lista preferiti |

#### Route solo admin

| Metodo | Percorso | Descrizione |
|--------|----------|-------------|
| GET | `/settings` | Pagina impostazioni |
| PUT | `/api/settings` | Salva impostazioni auto-scan |
| POST | `/api/settings/scan` | Forza scansione |
| PUT | `/api/settings/scanner-depth` | Imposta profondità minima |
| PUT | `/api/settings/ignored-folders` | Imposta cartelle ignorate |
| POST | `/api/settings/ignored-folders/add` | Aggiungi cartella ignorata |
| PUT | `/api/settings/excluded-folders` | Imposta cartelle escluse |
| DELETE | `/api/settings/excluded-paths` | Rimuovi percorso escluso |
| POST | `/api/settings/users` | Crea utente |
| DELETE | `/api/settings/users/{id}` | Elimina utente |
| POST | `/api/settings/users/{id}/roles` | Assegna ruolo |
| DELETE | `/api/settings/users/{id}/roles/{roleId}` | Rimuovi ruolo |
| GET | `/feedback` | Pagina admin feedback |
| GET | `/api/feedback` | Lista feedback |
| POST | `/api/feedback` | Invia feedback |
| GET | `/api/feedback/modal` | Modale form feedback |
| PUT | `/api/feedback/{id}/status` | Aggiorna stato |
| DELETE | `/api/feedback/{id}` | Elimina feedback |
| GET/POST/PUT/DELETE | `/api/feedback/categories/*` | Gestione categorie |

## Guida Utente

### Primo avvio

1. Avvia l'applicazione e apri `http://localhost:8080`
2. Verrai reindirizzato alla pagina di **Setup** — crea l'account admin
3. Effettua il login con le credenziali appena create
4. Vai in **Impostazioni** > **Scanner** e clicca **Forza Scansione** per indicizzare i tuoi modelli

### Navigazione dei modelli

- Usa le **tab delle categorie** in alto per filtrare per categoria principale
- La **sidebar** mostra le sotto-categorie quando una categoria è selezionata
- Usa la **barra di ricerca** per trovare modelli per nome (ricerca full-text)
- Clicca la **stella** su qualsiasi card modello per aggiungerlo ai preferiti

### Gestione di un modello

Nella pagina dettaglio modello puoi:
- Visualizzare il modello 3D nel **viewer integrato** (naviga tra i file usando le tab)
- Sfogliare la **galleria immagini** e aprire le immagini in un lightbox
- Modificare il **nome** e le **note**
- Assegnare un **autore** (ricerca typeahead, crea se non trovato)
- Assegnare una **categoria** (ricerca typeahead)
- Aggiungere/rimuovere **tag** (ricerca typeahead, crea se non trovato)
- Attivare/disattivare la **visibilità** (i modelli nascosti appaiono sfumati nella griglia)
- **Unire** con un altro modello (sposta tutti i file e i tag)
- **Eliminare** il modello (rimuove i file dal disco permanentemente)

### Impostazioni (Admin)

La pagina Impostazioni ha tre tab:

- **Scanner** — visualizza l'ultima scansione, forza scansione, abilita scansione giornaliera programmata, imposta profondità minima
- **Percorsi** — configura nomi cartelle ignorate, cartelle escluse, visualizza/gestisci percorsi esclusi
- **Utenti** — lista utenti, assegna/rimuovi ruoli, crea nuovi utenti

### Lingua

Clicca **IT** o **EN** nella navbar per cambiare lingua. La preferenza viene salvata in un cookie e persiste tra le sessioni.
