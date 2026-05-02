// Package drive wraps the Drive v3 client for the single use paymint has —
// reading the spreadsheet's `version` field to detect concurrent edits during
// a sync run (Red Team F11).
package drive

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// VersionGetter exposes the single Drive operation sync needs. Backed by
// Service in production; tests use a fake.
type VersionGetter interface {
	GetVersion(ctx context.Context, fileID string) (int64, error)
}

// Service wraps drive.Service.
type Service struct {
	svc *drive.Service
}

// NewService constructs a Drive v3 client.
func NewService(ctx context.Context, hc *http.Client) (*Service, error) {
	svc, err := drive.NewService(ctx, option.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("drive.NewService: %w", err)
	}
	return &Service{svc: svc}, nil
}

// GetVersion returns Drive's monotonic version counter for the file. Sync
// reads this twice: before pull and after push. A change between the two
// means a concurrent edit happened, and the algorithm re-runs.
func (s *Service) GetVersion(ctx context.Context, fileID string) (int64, error) {
	f, err := s.svc.Files.Get(fileID).Fields("version").Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("drive get %s: %w", fileID, err)
	}
	return f.Version, nil
}
