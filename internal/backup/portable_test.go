package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanonicalValuesRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		typeName string
		want     string
	}{
		{name: "null", in: nil, want: "null"},
		{name: "boolean", in: true, want: "boolean"},
		{name: "integer", in: int64(42), want: "integer"},
		{name: "decimal", in: "12.3400", typeName: "numeric", want: "decimal"},
		{name: "driver decimal", in: []byte("12.3400"), typeName: "decimal", want: "decimal"},
		{name: "driver boolean", in: []byte("1"), typeName: "boolean", want: "boolean"},
		{name: "timestamp", in: mustTime("2026-01-02T03:04:05Z"), want: "timestamp"},
		{name: "bytes", in: []byte{0, 1, 255}, want: "bytes"},
		{name: "json", in: ` { "ok": true } `, typeName: "json", want: "json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeCanonical(tc.in, tc.typeName)
			if err != nil {
				t.Fatal(err)
			}
			if encoded.Type != tc.want {
				t.Fatalf("type = %q, want %q", encoded.Type, tc.want)
			}
			decoded, err := DecodeCanonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "bytes" && !bytes.Equal(decoded.([]byte), tc.in.([]byte)) {
				t.Fatalf("bytes changed")
			}
		})
	}
}

func TestPortableArchiveRoundTrip(t *testing.T) {
	archive := PortableArchive{Tables: []PortableTable{{Name: "users", Rows: []PortableRow{{"id": {Type: "integer", Value: "1"}}}}}}
	data, err := MarshalArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Tables[0].Rows[0]["id"].Value != "1" {
		t.Fatalf("unexpected archive: %#v", got)
	}
}

func TestPortableArchiveStoreVerifiesArtifact(t *testing.T) {
	dir := t.TempDir()
	want := PortableArchive{Tables: []PortableTable{{Name: "users"}}}
	if err := WritePortableArchive(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPortableArchive(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Tables[0].Name != "users" {
		t.Fatalf("unexpected portable archive: %#v", got)
	}
}

// TestPortableServiceCreateRejectsExistingBackupSet ensures timestamp collisions cannot overwrite a completed backup.
func TestPortableServiceCreateRejectsExistingBackupSet(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "existing")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewPortableService().Create(context.Background(), dir, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "create exclusive portable backup set") {
		t.Fatalf("create error = %v, want exclusive backup set rejection", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("existing backup set changed: %v", err)
	}
}

// TestPortableServiceCreateCleansFailedBackupSet ensures failed exports do not block an immediate retry.
func TestPortableServiceCreateCleansFailedBackupSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "portable-backup")
	_, err := NewPortableService().Create(context.Background(), dir, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "database connection is required") {
		t.Fatalf("create error = %v, want database validation failure", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("failed backup set stat error = %v, want not found", err)
	}
}

func TestPortableSQLDialectUsesDriverSyntax(t *testing.T) {
	mysql, err := NewSQLDialect("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if got := mysql.QuoteIdentifier("order"); got != "`order`" {
		t.Fatalf("mysql identifier = %q", got)
	}
	postgres, err := NewSQLDialect("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got := postgres.Placeholder(2); got != "$2" {
		t.Fatalf("postgres placeholder = %q", got)
	}
}

func TestMigrationFingerprintRejectsDifferentContracts(t *testing.T) {
	archive := PortableArchive{MigrationFingerprint: MigrationFingerprint([]string{"001_users", "002_events"})}
	if err := ValidateMigrationFingerprint(archive, MigrationFingerprint([]string{"001_users", "002_events"})); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMigrationFingerprint(archive, MigrationFingerprint([]string{"001_users", "003_events"})); err == nil {
		t.Fatal("expected migration fingerprint mismatch")
	}
}

func TestPortableSchemaCompatibilityRejectsMissingTargetTable(t *testing.T) {
	source := PortableArchive{Tables: []PortableTable{{Name: "users", Columns: []ColumnSpec{{Name: "id", Type: "integer"}}}}}
	if err := ValidateSchemaCompatibility(source, nil); err == nil {
		t.Fatal("expected missing target table error")
	}
}

// mustTime parses a fixed test timestamp.
func mustTime(value string) interface{} {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
