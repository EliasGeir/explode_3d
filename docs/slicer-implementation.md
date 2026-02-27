# Slicer DLP/SLA — Implementazione

## Overview

Il modulo Slicer consente di selezionare file STL dalla pagina dettaglio modello e inviarli a una sezione dedicata dove:
- Scegliere un profilo stampante resin Anycubic
- Personalizzare le impostazioni di stampa (layer height, exposure, ecc.)
- Visualizzare il modello sul piatto di stampa 3D (Three.js)
- Effettuare lo slicing e scaricare un file `.photon`

Lo slicing avviene **server-side in Go puro** (nessuna dipendenza esterna per il core).

## Architettura Pipeline

```
File STL
  |
  v
[1] STL Parser (internal/slicer/stl.go)
  |  Legge binary/ASCII STL -> Mesh (triangoli + bounding box)
  |  Centra il modello sul piatto (CenterOnPlate)
  v
[2] Slicer (internal/slicer/slice.go)
  |  Per ogni layer Z: intersezione piano-triangolo
  |  Produce segmenti -> linkati in contorni chiusi
  v
[3] Rasterizer (internal/slicer/raster.go)
  |  Contorni -> bitmap image.Gray (scanline fill, even-odd rule)
  |  Supporto anti-aliasing 2x/4x/8x via supersampling
  v
[4] RLE Encoder + Photon Writer (internal/slicer/photon.go)
  |  Bitmap -> RLE encoding (bit 7=colore, bits 0-6=run length)
  |  Scrive header + layer table + layer data in formato .photon
  v
[5] File .photon pronto per il download
```

## Formato .photon

Formato binario reverse-engineered (spec da Photonsters):

| Sezione | Offset | Descrizione |
|---------|--------|-------------|
| Header | 0-75 (76 byte) | Magic `0x12fd0086`, version, bed size, resolution, layer count, exposure params |
| Layer Table | dopo header | Array di entry (36 byte ciascuna): Z, offset dati, lunghezza, exposure |
| Layer Data | dopo tabella | Bitmap RLE-encoded per ogni layer |

### RLE Encoding
- Bit 7: colore (0=scuro/vuoto, 1=chiaro/pieno)
- Bits 0-6: lunghezza run (max 125 pixel)
- Scansione riga per riga (y poi x)

## Stampanti Supportate (Built-in)

| Modello | Volume (mm) | Risoluzione | Pixel (um) |
|---------|-------------|-------------|------------|
| Photon Mono | 130 x 80 x 165 | 2560 x 1620 | 51 |
| Photon Mono X | 192 x 120 x 245 | 3840 x 2400 | 50 |
| Photon Mono X 6Ks | 196 x 122 x 200 | 5760 x 3600 | 34 |
| Photon Ultra | 102.4 x 57.6 x 165 | 1280 x 720 | 80 |
| Photon M3 | 164 x 102 x 180 | 4096 x 2560 | 40 |
| Photon M3 Plus | 197 x 122 x 245 | 5760 x 3600 | 34 |
| Photon M3 Max | 298 x 164 x 300 | 6480 x 3600 | 46 |

E' possibile aggiungere profili custom dall'interfaccia.

## Database

### Tabella `printer_profiles`
```sql
id SERIAL PRIMARY KEY
name TEXT NOT NULL
manufacturer TEXT NOT NULL DEFAULT ''
build_width_mm, build_depth_mm, build_height_mm DOUBLE PRECISION
resolution_x, resolution_y INTEGER
pixel_size_um DOUBLE PRECISION
is_built_in BOOLEAN DEFAULT FALSE  -- profili precaricati non eliminabili
created_at TIMESTAMPTZ DEFAULT NOW()
```

### Tabella `print_settings`
```sql
id SERIAL PRIMARY KEY
name TEXT NOT NULL
profile_id INTEGER REFERENCES printer_profiles(id) ON DELETE CASCADE
layer_height_mm DOUBLE PRECISION DEFAULT 0.05
exposure_time_s DOUBLE PRECISION DEFAULT 2.0
bottom_exposure_s DOUBLE PRECISION DEFAULT 30.0
bottom_layers INTEGER DEFAULT 5
lift_height_mm DOUBLE PRECISION DEFAULT 6.0
lift_speed_mmps DOUBLE PRECISION DEFAULT 2.0
retract_speed_mmps DOUBLE PRECISION DEFAULT 4.0
anti_aliasing INTEGER DEFAULT 1
is_default BOOLEAN DEFAULT FALSE
created_at TIMESTAMPTZ DEFAULT NOW()
```

## API Endpoints

| Metodo | Path | Descrizione |
|--------|------|-------------|
| GET | `/slicer?files=1,2,3&model_id=5` | Pagina slicer con file selezionati |
| GET | `/api/slicer/profiles` | Lista profili stampante (HTML fragment) |
| POST | `/api/slicer/profiles` | Crea profilo custom |
| PUT | `/api/slicer/profiles/{id}` | Modifica profilo |
| DELETE | `/api/slicer/profiles/{id}` | Elimina profilo (solo custom) |
| GET | `/api/slicer/settings/{profileId}` | Carica impostazioni per profilo (HTMX swap) |
| PUT | `/api/slicer/settings/{id}` | Salva impostazioni |
| POST | `/api/slicer/slice` | Avvia job di slicing (ritorna progress bar) |
| GET | `/api/slicer/status/{jobId}` | Stato job (HTMX polling ogni 1s) |
| GET | `/api/slicer/download/{jobId}` | Download file .photon generato |

## Flusso Utente

1. Dalla pagina dettaglio modello, l'utente seleziona i file STL con le checkbox
2. Click su "Slice Selected" o sull'icona slice del singolo file
3. Viene reindirizzato a `/slicer?files=IDs&model_id=ID`
4. La pagina mostra il preview 3D del piatto con il modello caricato
5. L'utente seleziona il profilo stampante e personalizza i parametri
6. Click su "Start Slicing" -> job asincrono con progress bar (polling HTMX ogni 1s)
7. Al completamento, compare il bottone per il download del `.photon`
8. Il file temporaneo viene eliminato dopo il download (o dopo 30 min di timeout)

## Come Aggiungere un Nuovo Profilo Stampante

### Via interfaccia
Selezionare "Add Profile" nella pagina slicer e compilare i campi.

### Via seed nel codice
Aggiungere un'entry nell'array `profiles` in `internal/database/database.go`, funzione `seedPrinterProfiles()`:
```go
{"Nome Modello", larghezzaMM, profonditaMM, altezzaMM, risoluzioneX, risoluzioneY, pixelSizeUM},
```

Per aggiungere profili a DB esistenti, creare una funzione `ensure...` simile a `ensurePhotonUltra()`.

## File Coinvolti

```
internal/models/models.go          - Struct PrinterProfile, PrintSettings, SliceJob
internal/database/migrations.go    - Schema tabelle printer_profiles, print_settings
internal/database/database.go      - Seed profili Anycubic + migrazione Photon Ultra
internal/repository/slicer.go      - CRUD profili e impostazioni
internal/slicer/stl.go             - Parser STL (binary + ASCII)
internal/slicer/slice.go           - Intersezione piano-Z con mesh triangolare
internal/slicer/raster.go          - Rasterizzazione scanline -> bitmap
internal/slicer/photon.go          - RLE encoding + writer formato .photon
internal/slicer/engine.go          - Job asincroni con progress tracking
internal/handlers/slicer.go        - HTTP handlers
templates/slicer.templ              - Pagina slicer + componenti HTMX
templates/model_detail.templ        - Checkbox STL + bottoni Slice
templates/layout.templ              - Link Slicer nella navbar
static/js/slicer3d.js              - Preview 3D piatto di stampa (Three.js)
internal/i18n/locales/en.json       - Chiavi slicer.* in inglese
internal/i18n/locales/it.json       - Chiavi slicer.* in italiano
main.go                             - Registrazione route
```

## Note Tecniche

### Coordinate System
- **Server-side (Go)**: usa coordinate STL native (Z-up). Il modello viene centrato in XY sul piatto e Z-bottom portato a 0.
- **Client-side (Three.js)**: converte da Z-up a Y-up durante il parsing STL. `STL(x,y,z)` -> `Three.js(x, z, -y)`.

### Anti-Aliasing
Livelli 2x, 4x, 8x: il rasterizer produce un'immagine a risoluzione moltiplicata, poi fa downsampling con media dei blocchi pixel.

### Job Management
- Jobs gestiti in-memory con `sync.Mutex`
- Ogni job è una goroutine separata
- Cleanup automatico dopo 30 minuti
- Progress tracking granulare (per-layer)
