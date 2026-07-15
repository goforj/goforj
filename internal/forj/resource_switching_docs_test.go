package forj

import (
	"strings"
	"testing"
)

// TestResourceSwitchingDocumentationPreservesOperationalContract verifies generated guidance does not overpromise environment-only driver changes.
func TestResourceSwitchingDocumentationPreservesOperationalContract(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "cache",
			path: "internal/caches/README.md.tmpl",
			want: []string{"which of them are active", "restart or redeploy", "regeneration", "cold cache", "not copied"},
		},
		{
			name: "queue",
			path: "internal/queues/README.md.tmpl",
			want: []string{"which of them are active", "restart or redeploy", "build a new artifact", "drain its workers", "Outstanding jobs are not copied"},
		},
		{
			name: "events",
			path: "internal/events/README.md.tmpl",
			want: []string{"which of them are active", "restart or redeploy", "build a new artifact", "delivery, ordering, replay, and durability", "not durable event storage"},
		},
		{
			name: "database",
			path: "internal/database/README.md",
			want: []string{"which of them are active", "restart or redeploy", "build a new artifact", "migrate its schema and data", "does not translate schemas or copy database data"},
		},
		{
			name: "storage",
			path: "internal/storages/README.md.tmpl",
			want: []string{"which of them are active", "restart or redeploy", "build a new artifact", "migrate existing objects", "generated URLs", "public/private visibility"},
		},
		{
			name: "mail",
			path: "internal/mail/README.md.tmpl",
			want: []string{"which of them are active", "restart or redeploy", "build a new artifact", "provider credentials", "retry", "delivery semantics"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := templatesFS.ReadFile(test.path)
			if err != nil {
				t.Fatalf("read resource documentation template: %v", err)
			}
			for _, want := range test.want {
				if !strings.Contains(string(content), want) {
					t.Errorf("%s omitted switching guidance %q", test.path, want)
				}
			}
		})
	}
}
