# paymint — setup

End-to-end walkthrough from a clean machine to your first synced invoice.

## 1. Install

```bash
go install github.com/vanducng/paymint@v0.1.0
```

Requires Go 1.25+. Pre-built binaries for macOS and Linux land on
[GitHub Releases](https://github.com/vanducng/paymint/releases) once Phase 6
ships GoReleaser.

## 2. Create a Google Cloud OAuth client

paymint uses **your own** Google Cloud OAuth client — no shared client, no
verification cliff. One-time setup, takes ~5 minutes.

1. Visit <https://console.cloud.google.com/projectcreate> and create a project
   (e.g. `paymint-personal`).
2. **APIs & Services → OAuth consent screen** → User Type: *External*.
   - App name: `paymint`. Support email: your email.
   - Scopes: leave empty here; paymint requests them at runtime.
   - Test users: add your own Google account.
3. **APIs & Services → Library** → enable:
   - **Google Sheets API**
   - **Google Drive API**
4. **APIs & Services → Credentials → + Create Credentials → OAuth client ID**.
   - Application type: **Desktop app**. Name: `paymint`.
   - Click **Download JSON** — this is your `credentials.json`. Store it
     somewhere only you can read (e.g. `~/.secrets/google_credentials.json`,
     mode `0600`).

## 3. Create the spreadsheet

paymint expects a single Google Sheet you'll point it at. Two options:

- **Easy**: create a blank sheet at <https://sheets.new>. paymint creates the
  five tabs (Companies, Contracts, Invoices, InvoiceLines, Payments) on first
  sync.
- **Existing sheet**: any sheet you own works. If you opened it via a sharing
  link rather than creating it, **add it to My Drive** (File → Add shortcut to
  Drive). Otherwise Google's Drive API returns 404 on the revision-counter
  check, and paymint disables F11 concurrent-edit protection for that run.

Copy the spreadsheet ID from the URL:
`https://docs.google.com/spreadsheets/d/<SPREADSHEET_ID>/edit`.

## 4. Initialise the data dir

```bash
paymint init --data-dir ~/paymint-data
```

You'll be prompted for:

- **Sheet ID** — from step 3.
- **OAuth client ID** — `installed.client_id` from your `credentials.json`.
- **Issuer block** — your name, address, email, and bank details (rendered on
  every PDF).

Press Enter on any optional field; you can edit
`<data-dir>/.paymint/config.yaml` later.

For automatic git snapshots on every sync, also run:

```bash
git -C ~/paymint-data init
git -C ~/paymint-data config user.email "you@example.com"
git -C ~/paymint-data config user.name  "Your Name"
```

## 5. Sign in

```bash
paymint --data-dir ~/paymint-data \
  auth login --credentials ~/.secrets/google_credentials.json
```

A browser opens; consent the scopes (Sheets read/write, Drive metadata
read-only). The token caches at `~/Library/Application Support/paymint/token.json`
on macOS, `$XDG_CONFIG_HOME/paymint/token.json` on Linux.

`paymint auth status` prints expiry; `paymint auth logout` revokes and deletes
the cached token.

## 6. Add data and sync

```bash
paymint --data-dir ~/paymint-data company add  --slug abs --name "Adventure Bound Studio"
paymint --data-dir ~/paymint-data contract add --company abs --title "Consulting" \
        --rate '$85.00' --start 2026-04-01
paymint --data-dir ~/paymint-data invoice add --company abs \
        --issue 2026-04-02 --due 2026-04-17 \
        --line "2026-04-02,Explore the API,4" \
        --line "2026-04-04,Standup,0.5"
paymint --data-dir ~/paymint-data sync
```

The first `sync` creates the five sheet tabs and pushes every queued op. The
second `sync` is a no-op (op_ids already on the sheet).

## 7. Render a PDF

```bash
paymint --data-dir ~/paymint-data pdf --invoice INV-abs-202604
# wrote ~/paymint-data/exports/INV-abs-202604.pdf
```

For every invoice issued in a given month:

```bash
paymint --data-dir ~/paymint-data pdf --month 2026-04 --all
```

## OAuth scopes (what paymint asks for)

| Scope | Why |
| --- | --- |
| `spreadsheets` | read & write the user's Google Sheets |
| `drive.metadata.readonly` | check the file's revision counter to detect concurrent edits |

The plan's `drive.file` (per-file via picker) is too narrow for an existing
user-owned sheet — it only sees files the app itself created. v0.1 falls back
to the broader `spreadsheets` scope. v0.2 may restore `drive.file` with a
picker integration.

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| `not signed in — run 'paymint auth login'` | Run `paymint auth login --credentials <path>`. |
| `googleapi: Error 404: File not found` (during sync) | The sheet is reachable via sharing link only. Open it in your browser, then **File → Add shortcut to Drive → My Drive**. Sync's F11 retry is disabled until then but it still proceeds. |
| `Unable to parse range: Companies!A1:Z` | First-sync race; rerun `paymint sync`. paymint creates missing tabs via `EnsureTabs`. |
| `state mismatch` during login | A second login attempt landed before the first finished. Wait 2 minutes (the listener auto-shuts) and retry. |
| `data dir is not initialized` | Run `paymint init` first. |
| `git config user.email missing` (during sync) | Run `git config --global user.email …` or set inside the data dir. |
| Pre-existing pending ops won't drain | Check `~/paymint-data/.paymint/pending.yaml`. The Sheet's `op_id` column is the idempotency key — a failed push keeps the unprocessed tail in `pending.yaml` for the next sync. |
