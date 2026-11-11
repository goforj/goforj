package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestAuthSuiteRequiresIdentities preserves evidence even when a larger suite appears to pass.
func TestAuthSuiteRequiresIdentities(t *testing.T) {
	valid := testResult{Passed: []string{"auth::Existing", "auth::New"}}
	if err := requireAuthSuite(valid, []string{"auth::Existing"}); err != nil {
		t.Fatal(err)
	}
	if err := requireAuthSuite(valid, []string{"auth::Removed"}); err == nil {
		t.Fatal("accepted missing behavior")
	}
	if err := requireAuthSuite(valid, []string{"other::Existing"}); err == nil {
		t.Fatal("accepted wrong package")
	}
}

// TestAuthSuiteRejectsIncompleteEvidence prevents process success from masking failed or skipped events.
func TestAuthSuiteRejectsIncompleteEvidence(t *testing.T) {
	cases := map[string]testResult{
		"exit":               {ExitCode: 1, Passed: []string{"auth::Existing"}},
		"empty":              {},
		"unfinished test":    {Passed: []string{"auth::Existing"}, IncompleteTests: []string{"auth::Other"}},
		"unfinished package": {Passed: []string{"auth::Existing"}, IncompletePackages: []string{"other"}},
		"skip":               {Passed: []string{"auth::Existing"}, Skipped: []string{"auth::Skipped"}},
		"failed event":       {Passed: []string{"auth::Existing"}, Failed: []string{"auth::Broken"}},
		"package failure":    {Passed: []string{"auth::Existing"}, PackageFailures: []string{"auth"}},
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			if err := requireAuthSuite(evidence, nil); err == nil {
				t.Fatal("accepted incomplete suite")
			}
		})
	}
}

// TestAuthFailureRequiresEveryRegression prevents compiler errors or a failing parent from proving the intended regressions.
func TestAuthFailureRequiresEveryRegression(t *testing.T) {
	required := []string{"auth::Parent/replay", "auth::Parent/expiry"}
	valid := testResult{ExitCode: 1, Failed: required, PackageFailures: []string{"auth"}}
	if err := requireAuthFailure(valid, required); err != nil {
		t.Fatal(err)
	}
	cases := map[string]testResult{
		"success":            {Failed: required},
		"unfinished test":    {ExitCode: 1, Failed: required, IncompleteTests: []string{"auth::Other"}},
		"unfinished package": {ExitCode: 1, Failed: required, IncompletePackages: []string{"other"}},
		"timeout":            {ExitCode: 124, Failed: required},
		"compiler":           {ExitCode: 1, PackageFailures: []string{"auth"}},
		"parent":             {ExitCode: 1, Failed: []string{"auth::Parent"}},
		"partial":            {ExitCode: 1, Failed: required[:1]},
		"skip":               {ExitCode: 1, Failed: required, Skipped: []string{"auth::Other"}},
		"wrong package":      {ExitCode: 1, Failed: []string{"other::Parent/replay", "other::Parent/expiry"}},
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			if err := requireAuthFailure(evidence, required); err == nil {
				t.Fatal("accepted false behavioral evidence")
			}
		})
	}
	if err := requireAuthFailure(valid, nil); err == nil {
		t.Fatal("accepted empty required behaviors")
	}
}

// TestReplaceAuthMethodPreservesNeighbors proves AST positions handle nested braces and strings without consuming another method.
func TestReplaceAuthMethodPreservesNeighbors(t *testing.T) {
	source := []byte("package auth\n// MarkUsed retains owner context.\nfunc (r *OAuthStateRepo) MarkUsed(id int64, usedAt time.Time) error {\n if id > 0 { println(\"} and func\") }; return nil\n}\n// Neighbor is app-owned.\nfunc (r *OAuthStateRepo) Neighbor() string { return `owner` }\nfunc (r *OtherRepo) MarkUsed() string { return `other owner` }\n")
	replacement := []byte("func (r *OAuthStateRepo) MarkUsed(id int64, usedAt time.Time) error { return ErrInvalidOAuthState }")
	updated, err := replaceAuthMethod(source, "r", "OAuthStateRepo", "MarkUsed", replacement)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{"// MarkUsed retains owner context.", "// Neighbor is app-owned.", "return `owner`", "return `other owner`", "return ErrInvalidOAuthState"} {
		if !strings.Contains(string(updated), retained) {
			t.Fatalf("lost %q", retained)
		}
	}
	if strings.Contains(string(updated), "println") {
		t.Fatal("retained old body")
	}
}

// TestReplaceAuthMethodRefusesUnsupportedShapes requires a single exact supported receiver and unchanged signature.
func TestReplaceAuthMethodRefusesUnsupportedShapes(t *testing.T) {
	replacement := []byte("func (r *Repo) Claim(id int64) error { return nil }")
	cases := map[string]string{
		"missing":             "package auth\nfunc Other() {}",
		"duplicate":           "package auth\nfunc (r *Repo) Claim(id int64) error {return nil}\nfunc (r *Repo) Claim(id int64) error {return nil}",
		"wrong receiver name": "package auth\nfunc (other *Repo) Claim(id int64) error {return nil}",
		"wrong receiver type": "package auth\nfunc (r *Other) Claim(id int64) error {return nil}",
		"value receiver":      "package auth\nfunc (r Repo) Claim(id int64) error {return nil}",
		"generic receiver":    "package auth\nfunc (r *Repo[T]) Claim(id int64) error {return nil}",
		"signature":           "package auth\nfunc (r *Repo) Claim(id string) error {return nil}",
		"bodyless":            "package auth\nfunc (r *Repo) Claim(id int64) error",
		"syntax":              "package auth\nfunc (r *Repo) Claim(",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := replaceAuthMethod([]byte(source), "r", "Repo", "Claim", replacement); err == nil {
				t.Fatal("accepted unsupported method shape")
			}
		})
	}
	if _, err := replaceAuthMethod([]byte("package auth\nfunc (r *Repo) Claim(id int64) error {return nil}"), "r", "Repo", "Claim", []byte("func Bad() {}")); err == nil {
		t.Fatal("accepted wrong replacement")
	}
}

// TestAuthReplacementAlwaysRestores preserves workflow bytes after rejected evidence or infrastructure errors.
func TestAuthReplacementAlwaysRestores(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "service.go")
			original := []byte("owner workflow\n")
			if err := os.WriteFile(path, original, 0640); err != nil {
				t.Fatal(err)
			}
			trialErr := errors.New("negative control infrastructure failure")
			err := withAuthReplacement(path, []byte("destructive workflow"), func() error {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if string(content) != "destructive workflow" {
					t.Fatal("replacement not exercised")
				}
				if failure {
					return trialErr
				}
				return nil
			})
			if failure && !errors.Is(err, trialErr) {
				t.Fatalf("lost trial error: %v", err)
			}
			if !failure && err != nil {
				t.Fatal(err)
			}
			restored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(restored) != string(original) {
				t.Fatal("owner workflow not restored")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
				t.Fatal("mode changed")
			}
		})
	}
}

// TestAuthFingerprintsDetectUnexpectedEdits checks additions, modifications, and removals against the app boundary.
func TestAuthFingerprintsDetectUnexpectedEdits(t *testing.T) {
	root := t.TempDir()
	auth := filepath.Join(root, "internal/auth")
	if err := os.MkdirAll(auth, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oauth_state.go", "service.go", "owner_test.go"} {
		if err := os.WriteFile(filepath.Join(auth, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := authFingerprints(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auth, "oauth_state.go"), []byte("fix"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auth, "upgrade_probe_integration_test.go"), []byte("regression"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := authFingerprints(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := authChangedFiles(before, after); !reflect.DeepEqual(got, []string{"oauth_state.go", "upgrade_probe_integration_test.go"}) {
		t.Fatalf("unexpected changes: %v", got)
	}
	if err := os.Remove(filepath.Join(auth, "owner_test.go")); err != nil {
		t.Fatal(err)
	}
	after, err = authFingerprints(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := authChangedFiles(before, after); !reflect.DeepEqual(got, []string{"oauth_state.go", "owner_test.go", "upgrade_probe_integration_test.go"}) {
		t.Fatalf("deletion escaped: %v", got)
	}
}

// TestAuthFingerprintsRefuseSymlinks prevents a copied auth path from reaching external source bytes.
func TestAuthFingerprintsRefuseSymlinks(t *testing.T) {
	root := t.TempDir()
	auth := filepath.Join(root, "internal/auth")
	if err := os.MkdirAll(auth, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := authFingerprints(root); err == nil {
		t.Fatal("accepted empty auth directory")
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(auth, "source.go")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := authFingerprints(root); err == nil {
		t.Fatal("accepted auth symlink")
	}
}
