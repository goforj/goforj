package projectupgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// testSnapshot supplies a reproducible comparison reference without claiming application adoption.
func testSnapshot(source string) Snapshot {
	return Snapshot{Protocol: ProtocolVersion, Scope: "framework", Engine: Engine{Version: "v0.1.0"}, InputsDigest: Digest([]byte("inputs")), Artifacts: []Artifact{{Ownership: "framework-template", Path: "internal/auth/service.go", Source: source}}}
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
					baseline.Artifacts = append(baseline.Artifacts, Artifact{Ownership: "framework-template", Path: "internal/auth/new.go", Source: old})
					target.Artifacts = append(target.Artifacts, Artifact{Ownership: "framework-template", Path: "internal/auth/collision.go", Source: next})
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
	target.Artifacts = append(target.Artifacts, Artifact{Path: "internal/caches/accessors_gen.go", Ownership: "app-derived"})
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
	baseline.Artifacts = append(append([]Artifact{}, baseline.Artifacts...), Artifact{Ownership: "framework-template", Path: "internal/auth/retired.go", Source: "package auth\n"})
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
		{"invalid version UTF8", func(s *Snapshot) { s.Engine.Version = string([]byte{0xff}) }},
		{"invalid commit UTF8", func(s *Snapshot) { s.Engine.Commit = string([]byte{0xff}) }},
		{"invalid source UTF8", func(s *Snapshot) {
			s.Artifacts[0].Source = string([]byte{0xff})
			s.Artifacts[0].Digest = Digest([]byte{0xff})
		}},
		{"empty artifacts", func(s *Snapshot) { s.Artifacts = nil }},
		{"too many artifacts", func(s *Snapshot) { s.Artifacts = make([]Artifact, 16385) }},
		{"traversal", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/../../.env" }},
		{"absolute", func(s *Snapshot) { s.Artifacts[0].Path = "/internal/auth/service.go" }},
		{"windows separator", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/a\\b.go" }},
		{"nul", func(s *Snapshot) { s.Artifacts[0].Path = "internal/auth/a\x00.go" }},
		{"duplicate", func(s *Snapshot) { s.Artifacts = append(s.Artifacts, s.Artifacts[0]) }},
		{"oversize", func(s *Snapshot) { s.Artifacts[0].Source = strings.Repeat("x", (4<<20)+1) }},
		{"unsafe template", func(s *Snapshot) { s.Artifacts[0].Template = "../private" }},
		{"git metadata", func(s *Snapshot) { s.Artifacts[0].Path = ".git/config" }},
		{"unsafe mode", func(s *Snapshot) { s.Artifacts[0].Mode = 0o7777 }},
		{"unknown ownership", func(s *Snapshot) { s.Artifacts[0].Ownership = "overwrite" }},
		{"private source", func(s *Snapshot) { s.Artifacts[0].Ownership = "private" }},
		{"private managed path", func(s *Snapshot) { s.Artifacts[0].Path = ".env" }},
		{"opaque digest", func(s *Snapshot) {
			s.Artifacts[0].Source = ""
			s.Artifacts[0].Ownership = "seed"
			s.Artifacts[0].Digest = Digest(nil)
		}},
		{"digest length", func(s *Snapshot) { s.Artifacts[0].Digest = "short" }},
		{"digest bytes", func(s *Snapshot) { s.Artifacts[0].Digest = strings.Repeat("x", 64) }},
		{"source mismatch", func(s *Snapshot) { s.Artifacts[0].Digest = Digest([]byte("other source")) }},
		{"missing asset digest", func(s *Snapshot) { s.Artifacts[0].Ownership = "framework-asset"; s.Artifacts[0].Source = "" }},
		{"missing ownership", func(s *Snapshot) { s.Artifacts[0].Ownership = "" }},
		{"empty template source mismatch", func(s *Snapshot) { s.Artifacts[0].Source = ""; s.Artifacts[0].Digest = Digest([]byte("not empty")) }},
		{"raw asset source", func(s *Snapshot) {
			s.Artifacts[0].Ownership = "framework-asset"
			s.Artifacts[0].Digest = Digest([]byte(s.Artifacts[0].Source))
		}},
		{"prefix collision", func(s *Snapshot) { s.Artifacts = append(s.Artifacts, Artifact{Path: "internal", Ownership: "seed"}) }},
		{"case collision", func(s *Snapshot) {
			copy := s.Artifacts[0]
			copy.Path = strings.ToUpper(copy.Path)
			s.Artifacts = append(s.Artifacts, copy)
		}},
		{"git case alias", func(s *Snapshot) { s.Artifacts[0].Path = ".GIT/config" }},
		{"trailing dot alias", func(s *Snapshot) { s.Artifacts[0].Path = ".git./config" }},
		{"environment case alias", func(s *Snapshot) { s.Artifacts[0].Path = ".ENV" }},
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

// TestDeclarationHintsIncludeCompilerContracts catches changes hidden in package headers or actual directive comments.
func TestDeclarationHintsIncludeCompilerContracts(t *testing.T) {
	before := "//go:build linux\n\npackage old\n//go:embed old.txt\nvar Asset string\n"
	after := "//go:build windows\n\npackage changed\n//go:embed new.txt\nvar Asset string\n"
	changed, invalid := changedDeclarations(before, after)
	if invalid || !reflect.DeepEqual(changed, []string{"build-constraints", "compiler-directives", "package", "var Asset"}) {
		t.Fatalf("compiler contracts=%v invalid=%v", changed, invalid)
	}
	before = "package example\nconst Text = `\n//go:embed pretend.txt\n`\n"
	after = "package example\nconst Text = `\n//go:embed other.txt\n`\n"
	changed, invalid = changedDeclarations(before, after)
	if invalid || !reflect.DeepEqual(changed, []string{"const Text"}) {
		t.Fatalf("raw string treated as compiler directive: %v", changed)
	}
}

// TestAlreadyReconciledDeclarationsAreNotOverlaps avoids reporting a conflict when app and target made the identical change.
func TestAlreadyReconciledDeclarationsAreNotOverlaps(t *testing.T) {
	root := t.TempDir()
	baseline := testSnapshot("package auth\nfunc Login(){old()}\n")
	target := testSnapshot("package auth\nfunc Login(){fixed()}\n")
	writeFixture(t, root, "internal/auth/service.go", target.Artifacts[0].Source)
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files[0].Overlaps) != 0 || plan.Files[0].Action != "matches-target" {
		t.Fatalf("resolved declaration reported as overlapping: %+v", plan.Files[0])
	}
}

// TestRequiredAndRetiredDerivedArtifactsRemainVisible avoids hiding required new output or deleting output from a disabled generator.
func TestRequiredAndRetiredDerivedArtifactsRemainVisible(t *testing.T) {
	root := t.TempDir()
	target := testSnapshot("package auth\n")
	target.Artifacts = append(target.Artifacts, Artifact{Path: "internal/events/accessors_gen.go", Ownership: "app-derived"}, Artifact{Path: "containers/observability/vmagent/metrics-targets.json", Ownership: "conditional-derived"})
	baseline := testSnapshot("package auth\n")
	baseline.Artifacts = append(baseline.Artifacts, Artifact{Path: "internal/queues/accessors_gen.go", Ownership: "app-derived"})
	writeFixture(t, root, "internal/queues/accessors_gen.go", "owner customized queue output")
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, file := range plan.Files {
		actions[file.Path] = file.Action
	}
	for name, action := range map[string]string{"internal/events/accessors_gen.go": "generate-derived-after-reconciliation", "internal/queues/accessors_gen.go": "review-retired-derived", "containers/observability/vmagent/metrics-targets.json": "review-derived-policy"} {
		if actions[name] != action {
			t.Fatalf("%s=%s, want %s", name, actions[name], action)
		}
	}
}

// TestExecutableOwnerDriftIsNotPristine ensures an upstream content change cannot hide an app-specific executable-bit change.
func TestExecutableOwnerDriftIsNotPristine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits")
	}
	root := t.TempDir()
	baseline := testSnapshot("package auth\nfunc Old(){}\n")
	baseline.Artifacts[0].Mode = 0o644
	target := testSnapshot("package auth\nfunc New(){}\n")
	target.Artifacts[0].Mode = 0o644
	writeFixture(t, root, "internal/auth/service.go", baseline.Artifacts[0].Source)
	if err := os.Chmod(filepath.Join(root, "internal/auth/service.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Action != "reconcile" || !plan.Files[0].ModeChanged {
		t.Fatalf("owner mode was treated as pristine: %+v", plan.Files[0])
	}
}

// TestBuildRejectsSpecialFiles prevents file and ancestor symlinks from redirecting inspection outside the app.
func TestBuildRejectsSpecialFiles(t *testing.T) {
	for _, kind := range []string{"ancestor", "file", "directory", "oversize", "parent-file"} {
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

// TestFrameworkOwnershipPoliciesSeparateRawAssetsSeedsSemanticAndDerived proves the planner is independent of any auth-specific inventory.
func TestFrameworkOwnershipPoliciesSeparateRawAssetsSeedsSemanticAndDerived(t *testing.T) {
	root := t.TempDir()
	target := testSnapshot("package auth\n")
	target.Artifacts = []Artifact{
		{Ownership: "framework-template", Path: "internal/http/server.go", Source: "package http\n", Mode: 0o644},
		{Path: "containers/logo.png", Ownership: "framework-asset", Digest: Digest([]byte{0xff, 0, 0xfe})},
		{Path: "cmd/app/frontend/src/App.vue", Ownership: "seed"},
		{Path: "app/worker/lifecycle.go", Ownership: "seed"},
		{Path: "go.mod", Ownership: "semantic"},
		{Path: ".env", Ownership: "private"},
		{Path: "internal/events/accessors_gen.go", Ownership: "app-derived"},
		{Path: "internal/generated-context.go", Ownership: "generated-context"},
	}
	writeFixture(t, root, "internal/http/server.go", "package http\n")
	if err := os.Chmod(filepath.Join(root, "internal/http/server.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "containers/logo.png", string([]byte{0xff, 0, 0xfe}))
	writeFixture(t, root, "cmd/app/frontend/src/App.vue", "owner content")
	writeFixture(t, root, ".env", "SECRET_CANARY=must-not-be-read")
	writeFixture(t, root, "internal/events/accessors_gen.go", "owner-specific generated output")
	plan, err := Build(root, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]File{}
	for _, file := range plan.Files {
		files[file.Path] = file
	}
	wanted := map[string]string{"internal/http/server.go": "review-mode", "containers/logo.png": "matches-target", "cmd/app/frontend/src/App.vue": "preserve-seed", "app/worker/lifecycle.go": "preserve-seed", "go.mod": "review-semantic", ".env": "preserve-private", "internal/events/accessors_gen.go": "regenerate-after-reconciliation", "internal/generated-context.go": "review-generated-context"}
	if runtime.GOOS == "windows" {
		wanted["internal/http/server.go"] = "matches-target"
	}
	for name, action := range wanted {
		if files[name].Action != action {
			t.Fatalf("%s action=%s, want %s", name, files[name].Action, action)
		}
	}
	if files[".env"].CurrentDigest != "" || files["go.mod"].CurrentDigest != "" || files["cmd/app/frontend/src/App.vue"].CurrentDigest != "" {
		t.Fatal("non-managed source was fingerprinted")
	}
}

// TestDerivedSymlinksAreNotRead keeps derived inspection separate from the later publication boundary.
func TestDerivedSymlinksAreNotRead(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "internal/auth/service.go", "package auth\n")
	if err := os.MkdirAll(filepath.Join(root, "internal/caches"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "absent-secret"), filepath.Join(root, "internal/caches/accessors_gen.go")); err != nil {
		t.Fatal(err)
	}
	target := testSnapshot("package auth\n")
	target.Artifacts = append(target.Artifacts, Artifact{Path: "internal/caches/accessors_gen.go", Ownership: "app-derived"})
	plan, err := Build(root, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		if file.Path == "internal/caches/accessors_gen.go" && (file.CurrentDigest != "" || file.Action != "regenerate-after-reconciliation") {
			t.Fatal("derived symlink was treated as source")
		}
	}
}

// TestFrameworkMigrationsRemainImmutableAndMissingOnesRequireReview distinguishes new schema work from rewriting deployed history.
func TestFrameworkMigrationsRemainImmutableAndMissingOnesRequireReview(t *testing.T) {
	root := t.TempDir()
	target := testSnapshot("package auth\n")
	target.Artifacts = []Artifact{{Path: "migrations/existing.up.sql", Ownership: "migration", Source: "new framework SQL"}, {Path: "migrations/new.up.sql", Ownership: "migration", Source: "new migration SQL"}}
	writeFixture(t, root, "migrations/existing.up.sql", "owner migration SQL")
	plan, err := Build(root, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		if file.Path == "migrations/existing.up.sql" && (file.Action != "preserve-migration" || file.CurrentDigest != "") {
			t.Fatal("existing migration history was compared as replaceable source")
		}
		if file.Path == "migrations/new.up.sql" && file.Action != "review-new-migration" {
			t.Fatal("new schema migration was hidden among owner seeds")
		}
	}
}

// TestFrameworkOwnershipTransitionsRequireExplicitReview prevents new renderer policies from silently claiming previously owner-seeded files.
func TestFrameworkOwnershipTransitionsRequireExplicitReview(t *testing.T) {
	root := t.TempDir()
	target := testSnapshot("package auth\n")
	baseline := testSnapshot("")
	baseline.Artifacts[0].Ownership = "seed"
	writeFixture(t, root, "internal/auth/service.go", "owner-controlled source")
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Action != "review-ownership-transition" || plan.Files[0].PreviousOwnership != "seed" || plan.Files[0].CurrentDigest != "" {
		t.Fatal("owner source was silently reclassified")
	}
}

// TestRetiredMigrationsCannotBecomeNewWork prevents a reference-only migration from being resurrected or removed.
func TestRetiredMigrationsCannotBecomeNewWork(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			root := t.TempDir()
			target := testSnapshot("package auth\n")
			baseline := testSnapshot("package auth\n")
			name := "migrations/retired.up.sql"
			baseline.Artifacts = append(baseline.Artifacts, Artifact{Path: name, Ownership: "migration", Source: "historical SQL"})
			if present {
				writeFixture(t, root, name, "app migration history")
			}
			plan, err := Build(root, target, &baseline)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range plan.Files {
				if file.Path != name {
					continue
				}
				found = true
				if !present || file.Action != "preserve-retired-migration" || file.CurrentDigest != "" {
					t.Fatalf("retired migration has unsafe policy: %+v", file)
				}
			}
			if present != found {
				t.Fatalf("retired migration presence=%v, planned=%v", present, found)
			}
		})
	}
}

// TestUnknownBaselineModeDoesNotHideOwnerChanges keeps absent reference metadata from treating an executable-bit change as pristine.
func TestUnknownBaselineModeDoesNotHideOwnerChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix executable bits")
	}
	root := t.TempDir()
	baseline := testSnapshot("package auth\nfunc Login(){old()}\n")
	target := testSnapshot("package auth\nfunc Login(){next()}\n")
	target.Artifacts[0].Mode = 0o644
	name := target.Artifacts[0].Path
	writeFixture(t, root, name, baseline.Artifacts[0].Source)
	if err := os.Chmod(filepath.Join(root, filepath.FromSlash(name)), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(root, target, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Action != "reconcile" || !plan.Files[0].ModeChanged {
		t.Fatalf("missing baseline mode concealed owner drift: %+v", plan.Files[0])
	}
}

// BenchmarkDeclarationHintsRepeatedInitializers exercises valid repeated declaration keys without quadratic fingerprint concatenation.
func BenchmarkDeclarationHintsRepeatedInitializers(b *testing.B) {
	source := "package app\n" + strings.Repeat("func init(){ record() }\n", 2500)
	b.ReportAllocs()
	for b.Loop() {
		if _, invalid := declarations(source); invalid {
			b.Fatal("valid repeated initializers were rejected")
		}
	}
}

// TestDeclarationHintsBindDirectivesToDeclarations detects compiler-policy movement even when the file-wide directive list is unchanged.
func TestDeclarationHintsBindDirectivesToDeclarations(t *testing.T) {
	before := "package app\n//go:noinline\nfunc First(){}\nfunc Second(){}\n"
	after := "package app\nfunc First(){}\n//go:noinline\nfunc Second(){}\n"
	changed, invalid := changedDeclarations(before, after)
	if invalid || !reflect.DeepEqual(changed, []string{"First", "Second"}) {
		t.Fatalf("directive movement lost declaration ownership: changed=%v invalid=%v", changed, invalid)
	}
}

// TestDeclarationHintsPreserveRepeatedInitializerOrder protects valid repeated keys while their fingerprints are assembled without repeated copying.
func TestDeclarationHintsPreserveRepeatedInitializerOrder(t *testing.T) {
	for _, test := range []struct {
		before, after, key string
	}{
		{"func init(){first()}\nfunc init(){second()}\n", "func init(){second()}\nfunc init(){first()}\n", "init"},
		{"var _ = first()\nvar _ = second()\n", "var _ = second()\nvar _ = first()\n", "var _"},
	} {
		changed, invalid := changedDeclarations("package app\n"+test.before, "package app\n"+test.after)
		if invalid || !reflect.DeepEqual(changed, []string{test.key}) {
			t.Fatalf("initializer order lost: changed=%v invalid=%v", changed, invalid)
		}
	}
}

// TestBuildCombinesContentAndModeChanges keeps upstream executable changes visible beside owner source edits.
func TestBuildCombinesContentAndModeChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits")
	}
	for _, test := range []struct {
		name               string
		old, next, current uint32
		action             string
	}{
		{"upstream mode and local content", 0o644, 0o755, 0o644, "reconcile"},
		{"local mode and content", 0o644, 0o644, 0o755, "preserve-local-relative-to-baseline"},
		{"upstream mode already applied", 0o644, 0o755, 0o755, "preserve-local-relative-to-baseline"},
		{"unknown baseline mode", 0, 0o644, 0o755, "reconcile"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			baseline := testSnapshot("original\n")
			baseline.Artifacts[0].Path = "tool.sh"
			baseline.Artifacts[0].Mode = test.old
			target := testSnapshot("original\n")
			target.Artifacts[0].Path = "tool.sh"
			target.Artifacts[0].Mode = test.next
			writeFixture(t, root, "tool.sh", "owner customization\n")
			if err := os.Chmod(filepath.Join(root, "tool.sh"), os.FileMode(test.current)); err != nil {
				t.Fatal(err)
			}
			plan, err := Build(root, target, &baseline)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Files[0].Action != test.action {
				t.Fatalf("action=%s, want %s", plan.Files[0].Action, test.action)
			}
		})
	}
}

// TestEmptyGoInputsRemainDiagnosed treats empty source as actual comparison data rather than an absent payload.
func TestEmptyGoInputsRemainDiagnosed(t *testing.T) {
	for _, kind := range []string{"target", "baseline", "current"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			baseline, target := testSnapshot("package auth\n"), testSnapshot("package auth\n")
			current := "package auth\n"
			switch kind {
			case "target":
				target.Artifacts[0].Source = ""
				current = ""
			case "baseline":
				baseline.Artifacts[0].Source = ""
			case "current":
				current = ""
			}
			writeFixture(t, root, target.Artifacts[0].Path, current)
			plan, err := Build(root, target, &baseline)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Files[0].InvalidGo {
				t.Fatalf("empty %s Go source was not diagnosed", kind)
			}
		})
	}
}

// TestSnapshotUnicodeRoundTrip preserves valid non-ASCII source and diagnostic metadata byte-for-byte.
func TestSnapshotUnicodeRoundTrip(t *testing.T) {
	snapshot := testSnapshot("package auth\n// Привет, 世界.\n")
	snapshot.Engine.Version = "dev-試験"
	snapshot.Engine.Commit = "référence"
	snapshot.Artifacts[0].Digest = Digest([]byte(snapshot.Artifacts[0].Source))
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, decoded) {
		t.Fatal("protocol changed valid Unicode source or metadata")
	}
}
