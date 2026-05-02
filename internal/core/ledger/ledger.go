// Package ledger holds the in-memory aggregate of every paymint entity and
// owns mutation tracking (dirty months) for the writer. No I/O, no clocks.
package ledger

import (
	"errors"
	"fmt"
	"sort"

	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/period"
)

// Ledger is the canonical container for all paymint state.
type Ledger struct {
	Companies map[string]*model.Company
	Contracts map[string]*model.Contract
	Invoices  map[string]*model.Invoice
	Payments  map[string]*model.Payment

	// dirty tracks which monthly shards a writer must rewrite. Companies and
	// contracts are flat (single file each), so a separate flag is used.
	dirtyInvoiceMonths map[period.YearMonth]struct{}
	dirtyPaymentMonths map[period.YearMonth]struct{}
	dirtyCompanies     bool
	dirtyContracts     bool
}

// New returns an empty Ledger ready for additions.
func New() *Ledger {
	return &Ledger{
		Companies:          make(map[string]*model.Company),
		Contracts:          make(map[string]*model.Contract),
		Invoices:           make(map[string]*model.Invoice),
		Payments:           make(map[string]*model.Payment),
		dirtyInvoiceMonths: make(map[period.YearMonth]struct{}),
		dirtyPaymentMonths: make(map[period.YearMonth]struct{}),
	}
}

// AddCompany inserts a company; returns an error if the ID already exists.
func (l *Ledger) AddCompany(c *model.Company) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if _, ok := l.Companies[c.ID]; ok {
		return fmt.Errorf("company %q already exists", c.ID)
	}
	l.Companies[c.ID] = c
	l.dirtyCompanies = true
	return nil
}

// AddContract inserts a contract; the referenced company must exist.
func (l *Ledger) AddContract(c *model.Contract) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if _, ok := l.Companies[c.CompanyID]; !ok {
		return fmt.Errorf("contract.company_id: unknown company %q", c.CompanyID)
	}
	if _, ok := l.Contracts[c.ID]; ok {
		return fmt.Errorf("contract %q already exists", c.ID)
	}
	l.Contracts[c.ID] = c
	l.dirtyContracts = true
	return nil
}

// AddInvoice inserts an invoice. Validates against the referenced company
// (slug match) and contract (rate fallback). Lines must already be populated.
func (l *Ledger) AddInvoice(inv *model.Invoice) error {
	company, ok := l.Companies[inv.CompanyID]
	if !ok {
		return fmt.Errorf("invoice.company_id: unknown company %q", inv.CompanyID)
	}
	var contractRate int64
	if inv.ContractID != "" {
		ct, ok := l.Contracts[inv.ContractID]
		if !ok {
			return fmt.Errorf("invoice.contract_id: unknown contract %q", inv.ContractID)
		}
		if ct.CompanyID != inv.CompanyID {
			return fmt.Errorf("invoice.contract_id: contract %q belongs to company %q, not %q",
				ct.ID, ct.CompanyID, inv.CompanyID)
		}
		contractRate = ct.DefaultRate
	}
	if err := inv.Validate(company.Slug, contractRate); err != nil {
		return err
	}
	if _, ok := l.Invoices[inv.ID]; ok {
		return fmt.Errorf("invoice %q already exists", inv.ID)
	}
	l.Invoices[inv.ID] = inv
	l.dirtyInvoiceMonths[period.FromDate(inv.IssueDate)] = struct{}{}
	return nil
}

// AddPayment inserts a payment; the parent invoice must exist and the new
// payment must not push the cumulative total over Σ payments + amount.
// Overpayment surfaces a non-fatal warning but is allowed (it is the CLI's
// job to surface to the user; the model permits it).
func (l *Ledger) AddPayment(p *model.Payment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if _, ok := l.Invoices[p.InvoiceID]; !ok {
		return fmt.Errorf("payment.invoice_id: unknown invoice %q", p.InvoiceID)
	}
	if _, ok := l.Payments[p.ID]; ok {
		return fmt.Errorf("payment %q already exists", p.ID)
	}
	l.Payments[p.ID] = p
	l.dirtyPaymentMonths[period.FromDate(p.Date)] = struct{}{}
	return nil
}

// MarkInvoiceStatus updates an invoice status and dirties its month.
func (l *Ledger) MarkInvoiceStatus(invoiceID string, status model.InvoiceStatus) error {
	inv, ok := l.Invoices[invoiceID]
	if !ok {
		return fmt.Errorf("invoice %q not found", invoiceID)
	}
	if !status.Valid() {
		return fmt.Errorf("invalid status %q", status)
	}
	inv.Status = status
	l.dirtyInvoiceMonths[period.FromDate(inv.IssueDate)] = struct{}{}
	return nil
}

// CrossValidate runs structural checks across the entire ledger after a load
// (duplicate detection at the loader layer covers most of this; this catches
// dangling references that survived a sheet-side edit).
func (l *Ledger) CrossValidate() error {
	var errs []error
	for id, c := range l.Contracts {
		if _, ok := l.Companies[c.CompanyID]; !ok {
			errs = append(errs, fmt.Errorf("contract %q references unknown company %q", id, c.CompanyID))
		}
	}
	for id, inv := range l.Invoices {
		if _, ok := l.Companies[inv.CompanyID]; !ok {
			errs = append(errs, fmt.Errorf("invoice %q references unknown company %q", id, inv.CompanyID))
		}
	}
	for id, p := range l.Payments {
		if _, ok := l.Invoices[p.InvoiceID]; !ok {
			errs = append(errs, fmt.Errorf("payment %q references unknown invoice %q", id, p.InvoiceID))
		}
	}
	return errors.Join(errs...)
}

// DirtyState describes which on-disk artifacts a writer must touch.
type DirtyState struct {
	Companies     bool
	Contracts     bool
	InvoiceMonths []period.YearMonth
	PaymentMonths []period.YearMonth
}

// Dirty returns the current dirty-tracker snapshot, sorted for determinism.
func (l *Ledger) Dirty() DirtyState {
	d := DirtyState{Companies: l.dirtyCompanies, Contracts: l.dirtyContracts}
	for ym := range l.dirtyInvoiceMonths {
		d.InvoiceMonths = append(d.InvoiceMonths, ym)
	}
	for ym := range l.dirtyPaymentMonths {
		d.PaymentMonths = append(d.PaymentMonths, ym)
	}
	sort.Slice(d.InvoiceMonths, func(i, j int) bool {
		return d.InvoiceMonths[i].Before(d.InvoiceMonths[j])
	})
	sort.Slice(d.PaymentMonths, func(i, j int) bool {
		return d.PaymentMonths[i].Before(d.PaymentMonths[j])
	})
	return d
}

// MarkClean clears all dirty trackers; called by the writer after a
// successful flush.
func (l *Ledger) MarkClean() {
	l.dirtyCompanies = false
	l.dirtyContracts = false
	l.dirtyInvoiceMonths = make(map[period.YearMonth]struct{})
	l.dirtyPaymentMonths = make(map[period.YearMonth]struct{})
}
