package sync

import (
	"fmt"

	yaml "github.com/goccy/go-yaml"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/sheets"
	"github.com/vanducng/paymint/internal/store/pending"
)

// mergeRow inserts a single sanitized row from a pulled tab into the ledger.
// Validation errors short-circuit the whole sync — the user wants a clear
// signal if the sheet has gone bad, not silent partial loads.
func mergeRow(l *ledger.Ledger, tab string, row []any) error {
	switch tab {
	case sheets.TabCompanies:
		c, _, err := sheets.RowToCompany(row)
		if err != nil {
			return err
		}
		// merge: existing entity is overwritten (sheet-canonical).
		if err := c.Validate(); err != nil {
			return err
		}
		l.Companies[c.ID] = c
		return nil

	case sheets.TabContracts:
		c, _, err := sheets.RowToContract(row)
		if err != nil {
			return err
		}
		if err := c.Validate(); err != nil {
			return err
		}
		l.Contracts[c.ID] = c
		return nil

	case sheets.TabInvoices:
		inv, _, err := sheets.RowToInvoice(row)
		if err != nil {
			return err
		}
		// Lines come from the InvoiceLines tab; insert as bare invoice.
		l.Invoices[inv.ID] = inv
		return nil

	case sheets.TabInvoiceLines:
		ln, _, err := sheets.RowToLine(row)
		if err != nil {
			return err
		}
		inv, ok := l.Invoices[ln.InvoiceID]
		if !ok {
			return fmt.Errorf("line %s references unknown invoice %q", ln.ID, ln.InvoiceID)
		}
		inv.Lines = append(inv.Lines, ln)
		// Re-derive total from line sum (defensive).
		inv.TotalCents += ln.AmountCents
		return nil

	case sheets.TabPayments:
		p, _, err := sheets.RowToPayment(row)
		if err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		l.Payments[p.ID] = p
		return nil
	}
	return fmt.Errorf("unknown tab %q", tab)
}

// opToRow converts a pending Op back into a (row, tab) pair for AppendRows.
// Returns ("", nil, nil) for ops that don't need an append (status flips —
// the sheet's status column is rewritten by the next pull).
//
// Op payloads land back as map[string]any after YAML decode, so the cleanest
// path is to re-marshal the payload and re-parse into the typed struct.
func opToRow(op pending.Op) (row []any, tab string, err error) {
	switch op.Kind {
	case pending.OpCompanyAdd:
		var c model.Company
		if err := remap(op.Payload, &c); err != nil {
			return nil, "", err
		}
		return sheets.CompanyToRow(&c, op.OpID), sheets.TabCompanies, nil

	case pending.OpContractAdd:
		var c model.Contract
		if err := remap(op.Payload, &c); err != nil {
			return nil, "", err
		}
		return sheets.ContractToRow(&c, op.OpID), sheets.TabContracts, nil

	case pending.OpInvoiceAdd:
		var inv model.Invoice
		if err := remap(op.Payload, &inv); err != nil {
			return nil, "", err
		}
		// Don't push lines here — caller pushes them via OpInvoiceLineAdd
		// or builds them from inv.Lines on append. v0.1: we expand lines
		// inline so a single OpInvoiceAdd handles both. Done in the caller.
		return sheets.InvoiceToRow(&inv, op.OpID), sheets.TabInvoices, nil

	case pending.OpInvoiceLineAdd:
		var l model.InvoiceLine
		if err := remap(op.Payload, &l); err != nil {
			return nil, "", err
		}
		return sheets.LineToRow(&l, op.OpID), sheets.TabInvoiceLines, nil

	case pending.OpPaymentAdd:
		var p model.Payment
		if err := remap(op.Payload, &p); err != nil {
			return nil, "", err
		}
		return sheets.PaymentToRow(&p, op.OpID), sheets.TabPayments, nil

	case pending.OpInvoiceStatus, pending.OpInvoiceLineRm:
		// status flips and removes are reflected by re-pulling the local
		// shard on the next sync; no append happens for them.
		return nil, "", nil
	}
	return nil, "", fmt.Errorf("unknown op kind %q", op.Kind)
}

// remap converts an arbitrary YAML-decoded payload (map[string]any after a
// round-trip) into a typed struct. Cheap reflection via the YAML codec.
func remap(payload any, out any) error {
	b, err := yaml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("remap marshal: %w", err)
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return fmt.Errorf("remap unmarshal: %w", err)
	}
	return nil
}
