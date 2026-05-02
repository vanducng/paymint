// Package pdfdoc holds the data shape rendered by internal/pdf. It is pure —
// no maroto, no I/O — so the document layout can be snapshot-tested.
package pdfdoc

import "github.com/vanducng/paymint/internal/core/config"

// Statement is the rendered shape of one invoice PDF.
type Statement struct {
	Issuer       config.Issuer
	Bank         config.Bank
	Counterparty Counterparty
	Header       InvoiceHeader
	Lines        []Line
	TotalCents   int64
	TotalHours   float32
	Notes        string
	Footer       Footer
}

// Counterparty is the billed company's contact block.
type Counterparty struct {
	ID      string
	Name    string
	Address string
	Email   string
	TaxID   string
}

// InvoiceHeader is what shows in the document title / header rows.
type InvoiceHeader struct {
	InvoiceNo    string // INV-<COMPANY>-<YYYYMM>
	IssueDate    string // ISO 8601
	DueDate      string // ISO 8601
	PaymentTerms string // e.g. "Net 15"
	Status       string // "issued" / "paid" / ...
}

// Line is one row of the hours table.
type Line struct {
	Date        string // ISO 8601
	Ref         string
	Desc        string
	RateLabel   string // "$85.00/hr"
	HoursLabel  string // "4.0"
	AmountLabel string // "$340.00"
}

// Footer wraps the small print under the total row.
type Footer struct {
	GeneratedAt string // ISO timestamp UTC
	GitShortSHA string // optional; "" when not in a git repo
}
