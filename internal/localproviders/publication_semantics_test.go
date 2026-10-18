package localproviders

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyRejectsDotenvReferencesThatChangeUnselectedValues checks resolved publication values rather than assignment text alone.
func TestApplyRejectsDotenvReferencesThatChangeUnselectedValues(t *testing.T) {
	for _, expression := range []string{
		"DB_REPORTS_DRIVER=${DB_DRIVER}\n",
		"DB_REPORTS_DRIVER=$DB_DRIVER\n",
		"REPORTS_PROVIDER=${DB_DRIVER}\nDB_REPORTS_DRIVER=${REPORTS_PROVIDER}\n",
		"UNRELATED_SECRET=${DB_DRIVER}-private-value\n",
	} {
		t.Run(expression, func(t *testing.T) {
			root := fixture(t)
			raw := "DB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\n" + expression
			put(t, root, ".env", raw)
			s := openTest(t, root)
			values, err := s.Drivers(map[string]string{"DB_DRIVER": "sqlite"})
			if err != nil {
				t.Fatal(err)
			}
			err = s.Apply(values)
			if err == nil || !strings.Contains(err.Error(), "resolves differently") {
				t.Fatalf("expected publication mismatch, got %v", err)
			}
			if strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "synthetic-private-dsn") {
				t.Fatal("publication diagnostic exposed a value")
			}
			actual, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != raw {
				t.Fatal("rejected publication changed active settings")
			}
			for _, name := range []string{stateName, lockName, ".gitignore"} {
				if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatalf("rejected publication created %s: %v", name, err)
				}
			}
		})
	}
}

// TestRestoreRejectsNewDependentReferences preserves the recovery point when later interpolation would change an unrelated value.
func TestRestoreRejectsNewDependentReferences(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("LATER_SETTING=${DB_DRIVER}\n")...)
	put(t, root, ".env", string(raw))
	before, err := os.ReadFile(filepath.Join(root, stateName))
	if err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	if err := s.Restore(); err == nil || !strings.Contains(err.Error(), "LATER_SETTING resolves differently") {
		t.Fatalf("expected restore mismatch, got %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, stateName))
	if err != nil {
		t.Fatal(err)
	}
	active, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !bytes.Equal(raw, active) {
		t.Fatal("rejected restoration changed settings or the backup")
	}
}

// TestPublicationPreservesStableDotenvReferences keeps unrelated expressions and literal dollar signs byte-stable.
func TestPublicationPreservesStableDotenvReferences(t *testing.T) {
	root := fixture(t)
	text := "# owner\nAPP_NAME=Demo\nAPP_LABEL=${APP_NAME}\nLITERAL='${DB_DRIVER}'\nDB_DRIVER=mysql\nCACHE_DRIVER=redis\n"
	put(t, root, ".env", text)
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(actual), "# owner\nAPP_NAME=Demo\nAPP_LABEL=${APP_NAME}\nLITERAL='${DB_DRIVER}'\n") {
		t.Fatal("stable interpolation or literal text was rewritten")
	}
	if err := openTest(t, root).Restore(); err != nil {
		t.Fatal(err)
	}
}

// TestReviewedDocumentRejectsMalformedOutput fails before writes if an invalid assignment reaches the rendering boundary.
func TestReviewedDocumentRejectsMalformedOutput(t *testing.T) {
	s := openTest(t, fixture(t))
	s.env.chunks = append(s.env.chunks, chunk{text: "INVALID='private-unclosed-value\n"})
	if _, err := s.reviewedDocument(s.Current); err == nil || strings.Contains(err.Error(), "private-unclosed-value") {
		t.Fatalf("expected redacted parse failure, got %v", err)
	}
}

// TestPortablePreservesWhitespaceDSNTargets mirrors runtime selection before trimming a named or App DSN.
func TestPortablePreservesWhitespaceDSNTargets(t *testing.T) {
	for _, key := range []string{"DB_REPORTS_DRIVER", "ADMIN_DB_DRIVER", "ADMIN_DB_REPORTS_DRIVER"} {
		for _, whitespace := range []string{" ", "\t", " \t "} {
			t.Run(key+"/"+whitespace, func(t *testing.T) {
				root := fixture(t)
				prefix := strings.TrimSuffix(key, "DRIVER")
				put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=synthetic-service-dsn\n"+key+"=sqlite\n"+prefix+"DSN='"+whitespace+"'\n"+prefix+"SQLITE_DATABASE=existing.db\n")
				s := openTest(t, root)
				app := s.databaseScope(key)
				before := databaseTargetInScope(s.Current, key, app)
				if before.dsn != "" || before.path != "existing.db" {
					t.Fatal("planner did not resolve the existing runtime target")
				}
				values, err := s.Portable()
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Apply(values); err != nil {
					t.Fatal(err)
				}
				actual := openTest(t, root)
				if databaseTargetInScope(actual.Current, key, app) != before || actual.Current[prefix+"DSN"] != whitespace {
					t.Fatal("local preset changed the existing SQLite target or masking DSN")
				}
				if err := actual.Restore(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestDatabaseSettingWhitespaceBlocksRootFallback matches envOr for every connection setting, not just DSNs.
func TestDatabaseSettingWhitespaceBlocksRootFallback(t *testing.T) {
	for _, suffix := range []string{"DRIVER", "DSN", "SQLITE_DATABASE", "DATABASE", "HOST", "PORT", "USERNAME", "PASSWORD"} {
		values := map[string]string{"DB_" + suffix: "root", "DB_REPORTS_" + suffix: " "}
		source, actual := databaseSettingInScope(values, "DB_REPORTS_DRIVER", suffix, "")
		if source != "DB_REPORTS_"+suffix || actual != " " {
			t.Fatalf("%s unexpectedly fell back to root", suffix)
		}
	}
}
