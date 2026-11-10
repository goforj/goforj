package projectupgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testSnapshot supplies a reproducible comparison reference without claiming application adoption.
func testSnapshot(source string) Snapshot {
	return Snapshot{Protocol: ProtocolVersion, Scope: "auth", Engine: Engine{Version: "v0.1.0"}, InputsDigest: Digest([]byte("inputs")), Artifacts: []Artifact{{Path: "internal/auth/service.go", Source: source}}}
}

// writeFixture keeps all upgrade experiments outside repository working trees.
func writeFixture(t *testing.T, root, name, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBuildActionMatrix exercises missing, deleted, collided, pristine, customized, and unversioned artifacts.
func TestBuildActionMatrix(t *testing.T) {
	const old = "package auth\nfunc Login() { old() }\n"
	const local = "package auth\nfunc Login() { local() }\n"
	const next = "package auth\nfunc Login() { next() }\n"
	for _, test := range []struct {
		name, current, target, action string
		baseline, missing, noOld      bool
	}{
		{name: "unversioned match", current: next, target: next, action: "matches-target"},
		{name: "unversioned differs", current: local, target: next, action: "review-unversioned"},
		{name: "unversioned missing", target: next, missing: true, action: "review-missing"},
		{name: "pristine reference", current: old, target: next, baseline: true, action: "upstream-only-relative-to-baseline"},
		{name: "owner difference", current: local, target: old, baseline: true, action: "preserve-local-relative-to-baseline"},
		{name: "overlapping change", current: local, target: next, baseline: true, action: "reconcile"},
		{name: "already applied", current: next, target: next, baseline: true, action: "matches-target"},
		{name: "owner deletion", target: next, baseline: true, missing: true, action: "review-missing"},
		{name: "new path collision", current: local, target: next, baseline: true, noOld: true, action: "review-collision"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if !test.missing {
				writeFixture(t, root, "internal/auth/service.go", test.current)
			}
			target := testSnapshot(test.target)
			var baseline *Snapshot
			if test.baseline {
				value := testSnapshot(old)
				baseline = &value
				if test.noOld {
					baseline.Artifacts = append(baseline.Artifacts, Artifact{Path: "internal/auth/new.go", Source: old})
					target.Artifacts = append(target.Artifacts, Artifact{Path: "internal/auth/collision.go", Source: next})
					writeFixture(t, root, "internal/auth/collision.go", local)
				}
			}
			plan, err := Build(root, target, baseline)
			if err != nil {
				t.Fatal(err)
			}
			wanted := "internal/auth/service.go"
			if test.noOld {
				wanted = "internal/auth/collision.go"
			}
			found := false
			for _, file := range plan.Files {
				if file.Path == wanted {
					found = true
					if file.Action != test.action {
						t.Fatalf("action=%s, want %s", file.Action, test.action)
					}
				}
			}
			if !found {
				t.Fatal("missing planned artifact")
			}
			if test.baseline && plan.BaselineProvenance != "supplied-unverified" {
				t.Fatal("baseline incorrectly authenticated")
			}
			if !test.missing {
				current, err := os.ReadFile(filepath.Join(root, "internal/auth/service.go"))
				if err != nil || string(current) != test.current {
					t.Fatal("inspection mutated source")
				}
			}
		})
	}
}

// TestBuildPreservesNeighborsAndSeparatesDerivedOutput reproduces Ship metadata and Ditracker impersonation extension patterns.
func TestBuildPreservesNeighborsAndSeparatesDerivedOutput(t *testing.T) {
	root := t.TempDir()
	baseline := testSnapshot("package auth\ntype Service struct{}\nfunc (s *Service) Refresh() { original() }\n")
	target := testSnapshot("package auth\ntype Service struct{}\nfunc (s *Service) Refresh() { fixed() }\n")
	current := "package auth\ntype Service struct{}\nfunc (s *Service) Refresh() { original(); preserveActor() }\n"
	writeFixture(t, root, "internal/auth/service.go", current)
	writeFixture(t, root, "internal/auth/session_sign_in.go", "package auth\n// app-specific identity\n")
	writeFixture(t, root, "internal/auth/impersonation.go", "package auth\n// app-specific actor\n")
	writeFixture(t, root, "internal/caches/accessors_gen.go", "package caches\nfunc OrganizationCache() {}\n")
	writeFixture(t, root, "internal/auth/custom_gen.go", "package auth\n// unknown despite suffix\n")
	writeFixture(t, root, ".env", "SECRET_CANARY=never-export-this\n")
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, file := range plan.Files {
		actions[file.Path] = file.Action
		if file.Path == "internal/auth/service.go" && !reflect.DeepEqual(file.Overlaps, []string{"Service.Refresh"}) {
			t.Fatalf("overlaps=%v", file.Overlaps)
		}
		if file.Ownership == "app-derived" && (file.CurrentDigest != "" || len(file.TargetDifferences) != 0) {
			t.Fatal("derived bytes judged as framework drift")
		}
	}
	for _, name := range []string{"session_sign_in.go", "impersonation.go", "custom_gen.go"} {
		if actions["internal/auth/"+name] != "preserve" {
			t.Fatalf("neighbor %s not preserved", name)
		}
	}
	if actions["internal/caches/accessors_gen.go"] != "regenerate-after-reconciliation" {
		t.Fatal("accessor not separated")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET_CANARY") || strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "preserveActor()") {
		t.Fatal("plan exports environment, absolute paths, or app source")
	}
	second, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, second) {
		t.Fatal("inspection is nondeterministic")
	}
}

// TestBuildRetirementsAndConfigurationChangesNeverGuessOwnership preserves customized and missing retired files.
func TestBuildRetirementsAndConfigurationChangesNeverGuessOwnership(t *testing.T) {
	root := t.TempDir()
	target := testSnapshot("package auth\n")
	baseline := target
	baseline.Artifacts = append(append([]Artifact{}, baseline.Artifacts...), Artifact{Path: "internal/auth/retired.go", Source: "package auth\n"})
	writeFixture(t, root, "internal/auth/retired.go", "package auth\nfunc Custom() {}\n")
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		if file.Path == "internal/auth/retired.go" && file.Action != "review-retirement" {
			t.Fatal("retired source is eligible for deletion")
		}
	}
	baseline.InputsDigest = Digest([]byte("other inputs"))
	if _, err := Build(root, target, &baseline); err == nil {
		t.Fatal("accepted mismatched inputs")
	}
	baseline = target
	baseline.Protocol = 99
	if _, err := Build(root, target, &baseline); err == nil {
		t.Fatal("accepted invalid baseline")
	}
}

// TestValidateSnapshotRejectsUnsafeContracts covers every snapshot validation branch before filesystem access.
func TestValidateSnapshotRejectsUnsafeContracts(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*Snapshot)
	}{
		{"protocol", func(s *Snapshot) { s.Protocol = 99 }},
		{"scope", func(s *Snapshot) { s.Scope = "all" }},
		{"digest length", func(s *Snapshot) { s.InputsDigest = "short" }},
		{"digest encoding", func(s *Snapshot) { s.InputsDigest = strings.Repeat("x", 64) }},
		{"engine", func(s *Snapshot) { s.Engine.Version = "" }},
		{"empty artifacts", func(s *Snapshot) { s.Artifacts = nil }},
		{"too many artifacts", func(s *Snapshot) { s.Artifacts = make([]Artifact, 257) }},
		{"traversal", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/../../.env" }},
		{"absolute", func(s *Snapshot) { s.Artifacts[0].Path = "/internal/auth/service.go" }},
		{"windows separator", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/a\\b.go" }},
		{"nul", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/a\x00.go" }},
		{"duplicate", func(s *Snapshot) { s.Artifacts = append(s.Artifacts, s.Artifacts[0]) }},
		{"oversize", func(s *Snapshot) { s.Artifacts[0].Source = strings.Repeat("x", (4<<20)+1) }},
		{"missing service", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/user.go" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testSnapshot("package auth\n")
			test.alter(&s)
			if err := ValidateSnapshot(s); err == nil {
				t.Fatal("accepted unsafe snapshot")
			}
			if _, err := Build(t.TempDir(), s, nil); err == nil {
				t.Fatal("build accepted unsafe snapshot")
			}
		})
	}
}

// TestBuildRejectsSpecialFiles prevents file and ancestor symlinks from redirecting inspection outside the app.
func TestBuildRejectsSpecialFiles(t *testing.T) {
	for _, kind := range []string{"ancestor", "file", "directory", "oversize", "parent-file", "derived-symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			writeFixture(t, root, "internal/auth/service.go", "package auth\n")
			var err error
			switch kind {
			case "ancestor":
				err = os.RemoveAll(filepath.Join(root, "internal/auth"))
				if err == nil {
					err = os.Symlink(outside, filepath.Join(root, "internal/auth"))
				}
			case "file":
				err = os.Remove(filepath.Join(root, "internal/auth/service.go"))
				if err == nil {
					err = os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "internal/auth/service.go"))
				}
			case "directory":
				err = os.Remove(filepath.Join(root, "internal/auth/service.go"))
				if err == nil {
					err = os.Mkdir(filepath.Join(root, "internal/auth/service.go"), 0o755)
				}
			case "oversize":
				writeFixture(t, root, "internal/auth/service.go", strings.Repeat("x", (4<<20)+1))
			case "parent-file":
				err = os.RemoveAll(filepath.Join(root, "internal/auth"))
				if err == nil {
					err = os.WriteFile(filepath.Join(root, "internal/auth"), []byte("owner"), 0o644)
				}
			case "derived-symlink":
				err = os.MkdirAll(filepath.Join(root, "internal/caches"), 0o755)
				if err == nil {
					err = os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "internal/caches/accessors_gen.go"))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Build(root, testSnapshot("package auth\n"), nil); err == nil {
				t.Fatal("accepted unsafe file")
			}
		})
	}
}

// TestDeclarationHintsHandleMalformedAndDisjointChanges avoids presenting syntax or non-overlap as merge safety.
func TestDeclarationHintsHandleMalformedAndDisjointChanges(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "internal/auth/service.go", "package auth\nfunc broken(")
	plan, err := Build(root, testSnapshot("package auth\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Files[0].InvalidGo {
		t.Fatal("malformed owner code not reported")
	}
	changed, invalid := changedDeclarations("package a\nimport \"a\"\ntype T int\nconst X=1\nvar Y=2\nfunc (*T) F(){}\n", "package a\nimport \"b\"\ntype T string\nconst X=2\nvar Y=3\nfunc (*T) F(){x()}\n")
	if invalid || len(changed) != 5 {
		t.Fatalf("declaration hints=%v, invalid=%v", changed, invalid)
	}
}

// TestDisjointDeclarationsStillRequireReconciliation rejects the inference that separate functions imply compatible behavior.
func TestDisjointDeclarationsStillRequireReconciliation(t *testing.T) {
	root := t.TempDir()
	baseline := testSnapshot("package auth\nfunc Request() { validate() }\nfunc EffectiveUser() { owner() }\n")
	target := testSnapshot("package auth\nfunc Request() { validate(); publishOwner() }\nfunc EffectiveUser() { owner() }\n")
	writeFixture(t, root, "internal/auth/service.go", "package auth\nfunc Request() { validate() }\nfunc EffectiveUser() { impersonate() }\n")
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Action != "reconcile" || len(plan.Files[0].Overlaps) != 0 {
		t.Fatalf("disjoint changes were treated as safely mergeable: %+v", plan.Files[0])
	}
}
