# Slicer Bugfix — Sessione 2

## Bug Riportati

1. **Mancava il profilo Anycubic Photon Ultra** nei preset
2. **Cambio profilo rompeva l'interfaccia** — il contenuto della pagina veniva re-renderizzato dentro la sezione settings
3. **Il file 3D non veniva renderizzato** nel viewer del slicer
4. **"Model has zero height"** quando si tentava lo slice

## Cause Radice e Fix

### Bug 1: Profilo Photon Ultra Mancante

**Fix**: Aggiunto `{"Photon Ultra", 102.4, 57.6, 165, 1280, 720, 80}` alla lista dei profili in `seedPrinterProfiles()`.

Per database esistenti (già seedati), aggiunta funzione `ensurePhotonUltra()` che inserisce il profilo solo se non esiste.

**Specifiche Photon Ultra**: DLP (non LCD), volume 102.4 x 57.6 x 165 mm, risoluzione 1280 x 720, pixel 80 um.

**File modificati**: `internal/database/database.go`

### Bug 2: Cambio Profilo Rompeva l'Interfaccia

**Causa**: Il `<select>` del profilo aveva `hx-get=""` come attributo HTMX. Con un URL vuoto, HTMX fa GET sulla URL corrente della pagina e inserisce l'intera pagina HTML dentro `#settings-form`.

**Fix**: Rimosso `hx-get=""`, `hx-swap`, `hx-target` dal `<select>`. Il cambio profilo era già gestito correttamente dal JavaScript inline che chiama `htmx.ajax('GET', '/api/slicer/settings/' + id, ...)`.

**File modificati**: `templates/slicer.templ` (componente `ProfileSelector`)

### Bug 3: File 3D Non Renderizzato

**Causa**: Mismatch nel sistema di coordinate. I file STL usano Z-up (Z è l'asse verticale), ma Three.js usa Y-up. Il viewer caricava le coordinate STL senza conversione, quindi il modello appariva "sdraiato" o invisibile.

**Fix**: Durante il parsing STL nel JavaScript, le coordinate vengono convertite:
```
STL (x, y, z) -> Three.js (x, z, -y)
```
- X rimane X
- Three.js Y (verticale) = STL Z (verticale)
- Three.js Z (profondità) = -STL Y

Inoltre corretto il centraggio del modello: ora centra in XZ e posiziona il bottom a Y=0 (sul piatto).

**File modificati**: `static/js/slicer3d.js`

### Bug 4: "Model has zero height"

**Cause multiple**:

1. **Impostazioni non inviate con il form HTMX**: Gli input delle impostazioni (layer_height, exposure, ecc.) usavano l'attributo HTML `form="slice-form"` per associarsi al form, ma HTMX non rispetta questo attributo — serializza solo gli input DOM-figli del form. Risultato: il server non riceveva `layer_height_mm` e usava il default dal DB.

2. **Formula Z-layer includeva MinBound[2] superfluo**: Dopo `CenterOnPlate`, `MinBound[2]` è già 0. La formula `i*layerHeight + layerHeight/2 + MinBound[2]` era corretta ma potenzialmente confusa.

3. **Classificazione vertici sul piano Z**: La classificazione usava `>` stretta che causava edge case quando vertici giacevano esattamente sul piano Z. Riscritto con approccio epsilon-based (1e-6 mm di tolleranza) e gestione esplicita dei 3 casi: sopra, sotto, sul piano.

**Fix**:
- Aggiunto `hx-include="#settings-form input, #settings-form select"` al form per includere gli input esterni
- Rimosso gli attributi `form="slice-form"` dagli input (non più necessari)
- Semplificata la formula Z: `float32(float64(i)*layerHeight + layerHeight/2)`
- Riscritto `intersectTrianglePlane` con classificazione a 3 stati (`+1`, `-1`, `0`) e epsilon
- Aggiunto messaggio di errore diagnostico con bounds e numero di triangoli

**File modificati**:
- `templates/slicer.templ` — hx-include, rimosso form= attributes
- `internal/slicer/engine.go` — formula Z, messaggio errore
- `internal/slicer/slice.go` — riscritta intersectTrianglePlane

## Lezioni Apprese

1. **HTMX non rispetta `form="..."` HTML attribute**: gli input associati via `form=` devono essere inclusi esplicitamente con `hx-include`.
2. **`hx-get=""` con URL vuoto è pericoloso**: HTMX interpreta una stringa vuota come la URL corrente della pagina, causando il fetch dell'intera pagina dentro un target parziale.
3. **Coordinate system mismatch** tra STL (Z-up) e Three.js (Y-up) è un classico problema che deve essere gestito sia lato client (visualizzazione) che lato server (slicing).
4. **Float comparison per intersezione geometrica** richiede epsilon, non confronti esatti.
