# paymint

> CLI for tracking received invoices, contracts, and payments — synced to a
> Google Sheet, snapshotted to git, exportable as per-invoice PDF statements.

**Status:** alpha (v0.1.0 in progress)

## Install

```bash
go install github.com/vanducng/paymint@latest
```

Requires Go 1.23 or newer.

## Quickstart

> Full setup walkthrough lands in `docs/setup.md` once Phase 6 ships.

```bash
paymint version
# paymint v0.1.0 (abc1234, 2026-05-03T10:00:00Z)
```

## Scope (v0.1)

- **USD-only** billing (multi-currency in v0.2)
- **Hourly line-item invoices** (matches consultant flow)
- Invoice IDs format: `INV-<COMPANY>-<YYYYMM>` (one per company per month)
- **Sheet-canonical** sync — Google Sheet is source of truth
- **Bring your own** Google Cloud OAuth client (see setup docs)

## License

MIT — see [LICENSE](./LICENSE).
