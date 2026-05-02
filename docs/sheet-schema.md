# paymint — sheet schema

paymint syncs to a Google Sheet over five tabs. Column order is **pinned** in
`internal/sheets/schema.go`; if you rename a tab or reorder columns by hand,
the next sync will refuse with `tab X has wrong header`.

The trailing **`op_id`** column carries the UUIDv4 of the originating CLI
operation. Sync uses it as an idempotency key — a row whose `op_id` is
already on the sheet is never re-appended.

## Companies

| col | name | type | notes |
| ---:|---|---|---|
| 1 | `id` | string | kebab-case, ≤64 chars |
| 2 | `slug` | string | lowercase; usually equals `id` |
| 3 | `name` | string | display name |
| 4 | `currency` | string | `USD` only in v0.1 |
| 5 | `tax_id` | string | optional |
| 6 | `address` | string | optional |
| 7 | `email` | string | optional |
| 8 | `op_id` | UUID | pending-op idempotency key |

## Contracts

| col | name | type | notes |
| ---:|---|---|---|
| 1 | `id` | string | kebab-case |
| 2 | `company_id` | string | foreign key → Companies.id |
| 3 | `title` | string | display label |
| 4 | `default_rate_cents` | int64 | hourly rate in cents (e.g. 8500 = $85.00) |
| 5 | `start` | date | ISO 8601 (`YYYY-MM-DD`) |
| 6 | `end` | date | ISO 8601, blank = open contract |
| 7 | `cadence` | string | optional label, e.g. `monthly` |
| 8 | `doc_url` | string | optional link |
| 9 | `notes` | string | optional |
| 10 | `op_id` | UUID | |

## Invoices

| col | name | type | notes |
| ---:|---|---|---|
| 1 | `id` | string | format `INV-<slug>-<YYYYMM>`, lowercase slug |
| 2 | `company_id` | string | foreign key → Companies.id |
| 3 | `contract_id` | string | optional foreign key → Contracts.id |
| 4 | `issue_date` | date | ISO 8601; YYYYMM must match the ID |
| 5 | `due_date` | date | ISO 8601 |
| 6 | `total_cents` | int64 | denormalised sum of line amounts |
| 7 | `status` | enum | `draft` \| `issued` \| `paid` \| `overdue` \| `revoked` |
| 8 | `notes` | string | optional |
| 9 | `op_id` | UUID | |

Status flips (e.g. `issued → paid`) are **not** appended as new invoice rows.
The next sync re-pulls the local YAML, which the writer rewrites in place.

## InvoiceLines

| col | name | type | notes |
| ---:|---|---|---|
| 1 | `id` | string | format `inv-<slug>-<YYYYMM>-l<NN>` |
| 2 | `invoice_id` | string | foreign key → Invoices.id |
| 3 | `date` | date | ISO 8601 |
| 4 | `description` | string | what the work was |
| 5 | `ref` | string | jira ticket / meeting reference |
| 6 | `rate_cents` | int64 | `0` = use contract's default rate |
| 7 | `hours` | float | 1-decimal display, full float32 precision stored |
| 8 | `amount_cents` | int64 | `round(effective_rate × hours)` |
| 9 | `op_id` | UUID | |

## Payments

| col | name | type | notes |
| ---:|---|---|---|
| 1 | `id` | string | format `inv-<slug>-<YYYYMM>-p<NN>` |
| 2 | `invoice_id` | string | foreign key → Invoices.id |
| 3 | `date` | date | ISO 8601 |
| 4 | `amount_cents` | int64 | per-payment amount |
| 5 | `method` | string | `wire` / `check` / `ach` / etc. |
| 6 | `reference` | string | bank reference / wire ID |
| 7 | `notes` | string | optional |
| 8 | `op_id` | UUID | |

## Editing the sheet by hand

The sheet is **canonical**: hand-edits flow back into local YAML on the next
`paymint sync`. Two safety rails:

1. **Pull-side sanitisation**. Cells starting with `=`, `+`, `-`, `@`, tab,
   or carriage return are prefixed with a leading `'` on read so they can't
   smuggle a formula into the local data or downstream PDFs.
2. **Cross-validation**. A row whose `company_id` / `invoice_id` foreign key
   doesn't resolve aborts the sync with a clear error. Fix the row or delete
   it before retrying.

## Why `op_id`?

If `paymint sync` crashes between appending row N and clearing
`pending.yaml`, the next run would otherwise re-append row N. Each pending
op carries a UUID; sync reads every existing `op_id` from the sheet and skips
ops already present. Drop / clear the column in the sheet only if you intend
to re-push from scratch — and only after wiping `.paymint/pending.yaml`.
