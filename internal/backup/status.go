package backup

import (
	"fmt"
	"time"
)

// Status describes the newest completed backup and its freshness.
type Status struct {
	Found     bool
	CreatedAt time.Time
	Age       time.Duration
	Resources int
}

// ReadStatus returns the newest locally completed backup under root.
func ReadStatus(root string, now time.Time) (Status, error) {
	manifests, err := List(root)
	if err != nil {
		return Status{}, err
	}
	if len(manifests) == 0 {
		return Status{}, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	created := manifests[0].CreatedAt
	return Status{Found: true, CreatedAt: created, Age: now.Sub(created), Resources: len(manifests[0].Resources)}, nil
}

// FormatStatus renders a compact operator-facing status line.
func FormatStatus(status Status) string {
	if !status.Found {
		return "backup status=missing"
	}
	return fmt.Sprintf("backup status=ok created_at=%s age=%s resources=%d", status.CreatedAt.UTC().Format(time.RFC3339), status.Age.Round(time.Second), status.Resources)
}
