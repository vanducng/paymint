package sheets

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
)

// Client is the minimal interface sync needs. Backed by Service in production
// and by a fake in tests so we can run the full algorithm offline.
type Client interface {
	EnsureTabs(ctx context.Context, spreadsheetID string, tabNames []string) error
	GetTab(ctx context.Context, spreadsheetID, tabName string) ([][]any, error)
	EnsureHeader(ctx context.Context, spreadsheetID, tabName string, header []string) error
	AppendRows(ctx context.Context, spreadsheetID, tabName string, rows [][]any) error
	UpdateRow(ctx context.Context, spreadsheetID, tabName string, rowIndex int, row []any) error
}

// Service is the production Sheets v4 client.
type Service struct {
	svc *sheets.Service
}

// NewService constructs a Sheets v4 client from an authorised HTTP client
// (e.g. cfg.Client(ctx, token)). All push paths use valueInputOption=RAW.
func NewService(ctx context.Context, hc *http.Client) (*Service, error) {
	svc, err := sheets.NewService(ctx, option.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("sheets.NewService: %w", err)
	}
	return &Service{svc: svc}, nil
}

// EnsureTabs adds any missing tabs to the spreadsheet via batchUpdate.
// Idempotent — existing tabs are skipped.
func (s *Service) EnsureTabs(ctx context.Context, spreadsheetID string, tabNames []string) error {
	meta, err := s.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet metadata: %w", err)
	}
	have := make(map[string]bool, len(meta.Sheets))
	for _, sh := range meta.Sheets {
		if sh != nil && sh.Properties != nil {
			have[sh.Properties.Title] = true
		}
	}
	var requests []*sheets.Request
	for _, name := range tabNames {
		if have[name] {
			continue
		}
		requests = append(requests, &sheets.Request{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: name},
			},
		})
	}
	if len(requests) == 0 {
		return nil
	}
	_, err = s.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("batch-update tabs: %w", err)
	}
	return nil
}

// GetTab returns every row of the named tab, header row included.
// Empty tabs return a nil slice.
func (s *Service) GetTab(ctx context.Context, spreadsheetID, tabName string) ([][]any, error) {
	rng := tabName + "!A1:Z"
	resp, err := s.svc.Spreadsheets.Values.Get(spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", tabName, err)
	}
	return resp.Values, nil
}

// EnsureHeader writes the canonical header row if the tab is empty. If the
// tab has rows but its header doesn't match, returns an error so the user
// can repair (we never auto-rewrite headers — that risks data corruption).
func (s *Service) EnsureHeader(ctx context.Context, spreadsheetID, tabName string, header []string) error {
	got, err := s.GetTab(ctx, spreadsheetID, tabName)
	if err != nil {
		return err
	}
	if len(got) == 0 {
		row := make([]any, len(header))
		for i, h := range header {
			row[i] = h
		}
		return s.AppendRows(ctx, spreadsheetID, tabName, [][]any{row})
	}
	if !headersEqual(got[0], header) {
		return fmt.Errorf("tab %s has wrong header (got %v, want %v) — fix the sheet manually",
			tabName, got[0], header)
	}
	return nil
}

// AppendRows appends rows to the bottom of the tab. valueInputOption=RAW
// (Red Team F8 — never USER_ENTERED).
func (s *Service) AppendRows(ctx context.Context, spreadsheetID, tabName string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	body := &sheets.ValueRange{Values: rows}
	_, err := s.svc.Spreadsheets.Values.
		Append(spreadsheetID, tabName+"!A1", body).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("append %s: %w", tabName, err)
	}
	return nil
}

// UpdateRow rewrites a specific row (1-indexed) with valueInputOption=RAW.
// Used to flip invoice status without re-appending.
func (s *Service) UpdateRow(ctx context.Context, spreadsheetID, tabName string, rowIndex int, row []any) error {
	rng := fmt.Sprintf("%s!A%d", tabName, rowIndex)
	body := &sheets.ValueRange{Values: [][]any{row}}
	_, err := s.svc.Spreadsheets.Values.
		Update(spreadsheetID, rng, body).
		ValueInputOption("RAW").
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update %s row %d: %w", tabName, rowIndex, err)
	}
	return nil
}

func headersEqual(got []any, want []string) bool {
	if len(got) < len(want) {
		return false
	}
	for i, w := range want {
		if FieldString(got, i) != w {
			return false
		}
	}
	return true
}
