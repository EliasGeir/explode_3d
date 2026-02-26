# Fix: Funzionalità Cancellazione Modello

## Stato attuale

La funzionalità di cancellazione esiste già ma richiede all'utente di digitare il nome del modello per confermare. Non c'è feedback visivo (spinner) durante l'operazione.

**File coinvolti:**
- `templates/merge.templ` — componenti `DeleteDialog` e script `deleteConfirm`
- `internal/handlers/models.go` — handler `DeleteModel`

## Flusso desiderato

1. L'utente accede al dettaglio del modello
2. Preme il pulsante "Delete model"
3. Appare un dialog con **Conferma** e **Annulla** (senza dover digitare il nome)
4. Se conferma → appare uno **spinner** nel dialog
5. In background: la cartella viene cancellata fisicamente dal disco (`os.RemoveAll`)
6. Il modello viene eliminato dal database (tabelle `model_files`, `model_tags`, `models`)
7. Lo spinner scompare e l'utente viene reindirizzato alla home page

## Modifiche da apportare

### 1. Template — `templates/merge.templ`

**Componente `DeleteDialog`** (righe 107-167): semplificare il dialog.

- Rimuovere l'input di testo per digitare il nome del modello
- Rimuovere la logica JS di validazione input → abilitazione bottone
- Aggiungere due bottoni: "Conferma eliminazione" (rosso) e "Annulla" (grigio)
- Aggiungere un elemento spinner nascosto (`#delete-spinner`) che viene mostrato al click di conferma
- I bottoni vengono nascosti quando appare lo spinner

**Script `deleteConfirm`** (righe 169-191): semplificare.

- Rimuovere il check sull'input (non serve più)
- Mostrare lo spinner prima della fetch
- Nascondere i bottoni
- Al completamento della fetch: redirect a `/`
- In caso di errore: nascondere spinner, rimostrare bottoni, mostrare alert

### 2. Handler — `internal/handlers/models.go`

**`DeleteModel`** (righe 571-612):

- Rimuovere il controllo `confirm_name` (non serve più, la conferma è tramite dialog)
- Rimuovere `r.ParseForm()` e `r.FormValue("confirm_name")`
- Il resto della logica rimane identico (delete filesystem → delete DB → redirect)

### 3. Rigenerazione templ

Dopo le modifiche a `merge.templ`, eseguire:

```bash
make generate
```

## Riepilogo file modificati

| File | Modifica |
|------|----------|
| `templates/merge.templ` | Dialog semplificato con conferma/annulla + spinner |
| `templates/merge_templ.go` | Rigenerato automaticamente da templ |
| `internal/handlers/models.go` | Rimossa validazione `confirm_name` |