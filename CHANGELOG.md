# Changelog

All notable changes to paymint are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] — 2026-05-03

First public release. Single-binary CLI for tracking received invoices,
contracts, and payments against a Google Sheet.

### Scope

- USD-only billing (multi-currency deferred to v0.2).
- Hourly line-item invoices — invoice ID `INV-<slug>-<YYYYMM>`, one invoice
  per (company, month).
- Sheet-canonical sync — Google Sheet is the source of truth; CLI writes go
  through a local `.paymint/pending.yaml` queue with UUIDv4 `op_id`
  idempotency keys, sync flushes them.
- User registers their own Google Cloud OAuth client (no shared client, no
  Google App Verification cliff).
- Single issuer per install — your name + bank details in
  `.paymint/config.yaml` ship onto every PDF.

### Added

- **CLI**: `paymint init`, `version`, `auth login | status | logout`,
  `company add | list`, `contract add | list`, `invoice add`,
  `invoice list`, `invoice mark-paid`, `invoice line add | remove`,
  `payment list`, `sync`, `pdf`.
- **OAuth desktop flow** with PKCE S256 + 32-byte state CSRF on a 127.0.0.1
  loopback. 2-minute deadline. Single-callback handler. Static success page
  (no user-controlled HTML).
- **Token storage** at `os.UserConfigDir()/paymint/token.json` (mode 0600).
  Refresh path holds a sibling flock to prevent concurrent refresh races.
- **Sheets v4 client** over five tabs (`Companies`, `Contracts`, `Invoices`,
  `InvoiceLines`, `Payments`). Push always uses `valueInputOption=RAW`.
  Pull-side sanitiser prefixes `'` to cells starting with `=`, `+`, `-`,
  `@`, tab, CR. Auto-creates missing tabs.
- **Drive v3 revision check** before pull and after push. Mismatch triggers
  up to 2 retry cycles.
- **Git snapshot** of the data dir after each sync. All git invocations via
  `exec.Command` (no shell), `--git-dir`/`--work-tree` pinned, commit
  message piped via stdin, only paths returned by `yamlstore.Save` staged.
  Cleanliness check refuses if dirty paths exist outside paymint's tracked
  top-level dirs.
- **PDF export** via maroto v2 — issuer block, bill-to block, hours table
  with dark header bar, totals row, payment-details + notes footer.

### Trade-offs

- OAuth scopes settled on `spreadsheets` + `drive.metadata.readonly`. The
  plan's original `drive.file` is too narrow for an existing user-owned
  sheet (it only sees files the app itself created or the user picks via
  Google's file picker). v0.2 may restore `drive.file` with picker
  integration.
- Drive's `files.get` returns 404 for sheets opened via a sharing link but
  not added to "My Drive". Sync logs a warning and proceeds without the
  F11 retry guard for that run. Add the sheet to "My Drive" to restore it.
- maroto pinned to v2.2.0; newer 2.4.0 requires Go 1.26.1.
- No diff-based dirty tracking on pull yet — we re-write every shard. Cheap
  on personal scale; revisit in v0.2 if datasets grow.

### Security

- All Red Team adjustments (F1, F4, F6, F7, F8, F9, F10, F11, F12, F13,
  F14) implemented. F6 partially relaxed (see trade-offs above).
- `govulncheck` runs on every CI build.

### Documentation

- `docs/setup.md` — end-to-end walkthrough including Cloud Console steps.
- `docs/sheet-schema.md` — column order per tab, op_id semantics.

### Known limitations

- No `paymint sync --dry-run` or `--no-commit` flags yet.
- No JSON output mode on `list` commands.
- Pagination on PDFs with very long line counts (>~25) relies on maroto's
  defaults; not stress-tested.
- Manual reconciliation only — paymint does not match bank-statement CSVs.
