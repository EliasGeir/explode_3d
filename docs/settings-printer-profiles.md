# Settings: Printer Profiles & Slicer Profile Persistence

## Panoramica

Aggiunta una sezione "Stampanti" (Printers) nella pagina Settings, accessibile a **tutti gli utenti autenticati** (non solo admin). Inoltre lo Slicer ora ricorda l'ultimo profilo selezionato dall'utente.

## Modifiche

### 1. Settings accessibili a tutti gli utenti

**Problema**: La pagina `/settings` era interamente nel gruppo di route admin-only. Gli utenti normali non potevano accedervi.

**Soluzione**: Spostata la route `GET /settings` nel gruppo protetto (tutti gli utenti autenticati). I tab admin-only (Scanner, Paths, Users) vengono mostrati solo se `IsAdmin == true`. Il tab "Printers" è sempre visibile.

- Per utenti normali: il tab di default è "printers"
- Per admin: il tab di default resta "scanner"
- Se un utente non-admin tenta di accedere a un tab admin, viene reindirizzato a "printers"

**File modificati**:
- `main.go` — spostata `r.Get("/settings", ...)` fuori dal gruppo admin
- `internal/handlers/settings.go` — logica di default tab e controllo accesso per tab

### 2. Tab Profili Stampante

Nuovo tab "Printers" in Settings con:

- **Lista profili esistenti**: mostra tutti i profili (built-in e custom) con dettagli (dimensioni, risoluzione, pixel size)
- **Badge**: "Built-in" per profili precaricati, "Custom" per quelli creati dall'utente
- **Modifica inline**: click su "Edit" espande un form inline sotto il profilo (solo per profili custom)
- **Eliminazione**: bottone "Delete" con conferma (solo per profili custom)
- **Creazione nuovo profilo**: form in fondo alla pagina con tutti i campi necessari (nome, produttore, dimensioni, risoluzione, pixel size)

Le API utilizzate sono quelle già esistenti dello Slicer:
- `POST /api/slicer/profiles` — crea profilo
- `PUT /api/slicer/profiles/{id}` — modifica profilo
- `DELETE /api/slicer/profiles/{id}` — elimina profilo

I handler riconoscono se la richiesta proviene dalla pagina Settings (tramite header `Hx-Current-Url`) e restituiscono il template appropriato (`PrinterProfilesList` invece di `ProfileSelector`).

**File modificati**:
- `templates/settings.templ` — aggiunto `PrinterProfiles` a `SettingsData`, tab "printers", componenti `printerProfilesTab`, `PrinterProfilesList`, `PrinterProfileForm`
- `internal/handlers/slicer.go` — handler CreateProfile/UpdateProfile/DeleteProfile ora restituiscono template diverso per settings vs slicer
- `internal/handlers/settings.go` — aggiunto `slicerRepo` a `SettingsHandler`, caricamento profili nel `Page()`

### 3. Persistenza Ultimo Profilo Selezionato

**Meccanismo**: Cookie `slicer_profile_id` con durata 1 anno.

**Flusso**:
1. Quando l'utente cambia profilo nello Slicer, il JavaScript scrive il cookie: `slicer_profile_id={id}`
2. Quando l'utente apre la pagina Slicer, il handler legge il cookie e verifica che il profilo esista ancora
3. Se il profilo nel cookie è valido, viene usato come default. Altrimenti si usa il primo profilo disponibile
4. Il profilo selezionato determina: le dimensioni del piatto 3D, le impostazioni di stampa caricate, e il `profile_id` nel form di slicing

**File modificati**:
- `templates/slicer.templ` — aggiunto `SelectedProfileID` a `SlicerPageData`, helper `selectedProfile()`, cookie JS in `ProfileSelector`
- `internal/handlers/slicer.go` — lettura cookie `slicer_profile_id` nel handler `Page()`

### 4. i18n

Nuove chiavi aggiunte in `settings.*`:
- `tab_printers` — nome tab
- `printer_profiles` — titolo sezione
- `printer_profiles_desc` — descrizione
- `add_printer_profile` — bottone aggiungi
- `no_printer_profiles` — messaggio lista vuota
- `built_in` — badge profilo predefinito

**File modificati**:
- `internal/i18n/locales/en.json`
- `internal/i18n/locales/it.json`

## Schema di accesso

| Tab | Admin | User |
|-----|-------|------|
| Scanner | Visibile | Nascosto |
| Paths | Visibile | Nascosto |
| Users | Visibile | Nascosto |
| Printers | Visibile | Visibile |
