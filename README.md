# paymint

> Track received invoices, contracts, and payments — synced to a Google Sheet,
> snapshotted to git, exportable as per-invoice PDFs.

**Status:** alpha · v0.1.0

paymint is a single-binary CLI for consultants and contractors. The Google
Sheet is the source of truth; the CLI is a fast recorder/reporter on top of
it. Local YAML mirror lets you `git diff` your billing history.

## Install

```bash
go install github.com/vanducng/paymint@v0.1.0
```

Requires Go 1.25+. Pre-built binaries land on
[Releases](https://github.com/vanducng/paymint/releases) once GoReleaser
publishes them.

## Quickstart

```bash
# 1. one-time Cloud Console setup → see docs/setup.md
# 2. initialise a data dir + config skeleton
paymint init --data-dir ~/paymint-data

# 3. sign in (PKCE flow opens browser)
paymint --data-dir ~/paymint-data \
  auth login --credentials ~/.secrets/google_credentials.json

# 4. add a company, a contract, an invoice with line items
paymint --data-dir ~/paymint-data company add  --slug abs --name "Adventure Bound Studio"
paymint --data-dir ~/paymint-data contract add --company abs --title "Consulting" \
        --rate '$85.00' --start 2026-04-01
paymint --data-dir ~/paymint-data invoice add --company abs \
        --issue 2026-04-02 --due 2026-04-17 \
        --line "2026-04-02,Explore the API,4" \
        --line "2026-04-04,Standup,0.5"

# 5. push to the sheet, snapshot the data dir
paymint --data-dir ~/paymint-data sync

# 6. mark paid, sync again, render the PDF
paymint --data-dir ~/paymint-data invoice mark-paid INV-abs-202604 \
        --date 2026-04-20 --amount '$382.50' --method wire
paymint --data-dir ~/paymint-data sync
paymint --data-dir ~/paymint-data pdf --invoice INV-abs-202604
# wrote ~/paymint-data/exports/INV-abs-202604.pdf
```

## Scope (v0.1)

- **USD-only** billing (multi-currency in v0.2).
- **Hourly line-item invoices** — invoice ID `INV-<slug>-<YYYYMM>`, one per
  company per month.
- **Sheet-canonical sync** — Google Sheet is the source of truth; CLI writes
  go through a local pending queue, sync flushes them with idempotent op_ids.
- **Bring your own** Google Cloud OAuth client (no shared client, no
  verification cliff).
- **Single issuer** — your billing details live in `.paymint/config.yaml` and
  ship onto every PDF.

## Architecture

Three-layer:

```
cmd/paymint  →  internal/cli (cobra)  →  internal/core (model, ledger)
                                       ↘  internal/store (yamlstore, pending, locks)
                                       ↘  internal/oauth, sheets, drive, snapshot, sync
                                       ↘  internal/pdf (maroto v2)
```

`internal/core/*` is pure Go — no I/O, no clocks. Adapters in
`internal/sheets`, `internal/drive`, `internal/oauth`, `internal/pdf`,
`internal/snapshot` keep the SDK churn at the edge. `internal/sync`
orchestrates the round-trip:

```
flock ./paymint/lock
preDriveVer = drive.GetVersion(spreadsheetID)
pull   5 tabs → ledger → write dirty shards
push   pending ops (skip op_ids already in sheet)
postDriveVer = drive.GetVersion(spreadsheetID)
if changed: pull/push again (max 2 retries)
git snapshot (only paths Save returned)
release flock
```

See `docs/setup.md`, `docs/sheet-schema.md`.

## Security model (high level)

- OAuth desktop flow uses **PKCE S256** + 32-byte state CSRF on a 127.0.0.1
  loopback (DNS rebinding-safe). Listener auto-closes after the first valid
  callback.
- Token cache at `os.UserConfigDir()/paymint/token.json` (mode 0600). Refresh
  path holds a flock so two paymint processes can't fight over a single-use
  refresh token.
- Push always uses `valueInputOption=RAW` — never `USER_ENTERED`. Pull-side
  sanitiser prefixes `'` to cells starting with `=`, `+`, `-`, `@`, tab, CR.
- All git invocations are via `exec.Command` (no shell), `--git-dir` /
  `--work-tree` pinned, commit message via stdin to defeat flag injection.

## License

MIT — see [LICENSE](./LICENSE).

## Contributing

Issues and PRs welcome. Run the smoke flow above before opening one.
