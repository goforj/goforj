// Package projectupgrade inspects framework upgrades without publishing application changes.
package projectupgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ProtocolVersion identifies the experimental renderer snapshot contract.
const ProtocolVersion = 1

// Snapshot contains only reproducible, non-secret template output, never application or environment contents.
type Snapshot struct {
	Protocol     int        `json:"protocol"`
	Scope        string     `json:"scope"`
	Engine       Engine     `json:"engine"`
	InputsDigest string     `json:"inputs_digest"`
	Artifacts    []Artifact `json:"artifacts"`
}

// Engine records actual binary provenance independently of the scaffold configuration version.
type Engine struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Dirty   bool   `json:"dirty"`
}

// Artifact is an incoming framework template, not a declaration of ownership over neighboring files.
type Artifact struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// Plan describes comparisons for review and deliberately carries no permission to apply them.
type Plan struct {
	Protocol           int      `json:"protocol"`
	Scope              string   `json:"scope"`
	Target             Engine   `json:"target"`
	Baseline           *Engine  `json:"baseline,omitempty"`
	BaselineProvenance string   `json:"baseline_provenance"`
	Files              []File   `json:"files"`
	Notes              []string `json:"notes"`
}

// File separates observed byte differences from unproven judgments about application intent.
type File struct {
	Path                string   `json:"path"`
	Ownership           string   `json:"ownership"`
	Action              string   `json:"action"`
	CurrentDigest       string   `json:"current_digest,omitempty"`
	BaselineDigest      string   `json:"baseline_digest,omitempty"`
	TargetDigest        string   `json:"target_digest,omitempty"`
	TargetDifferences   []string `json:"target_differences,omitempty"`
	LocalDifferences    []string `json:"local_differences,omitempty"`
	UpstreamDifferences []string `json:"upstream_differences,omitempty"`
	Overlaps            []string `json:"overlaps,omitempty"`
	InvalidGo           bool     `json:"invalid_go,omitempty"`
}

// Digest fingerprints exact output so reconstruction can be checked without storing baseline copies in an app.
func Digest(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

// ValidateSnapshot limits foreign renderer output to the advertised, bounded auth source surface.
func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.Protocol != ProtocolVersion || snapshot.Scope != "auth" {
		return fmt.Errorf("unsupported renderer snapshot protocol or scope")
	}
	if len(snapshot.InputsDigest) != 64 {
		return fmt.Errorf("renderer snapshot requires an input digest")
	}
	if _, err := hex.DecodeString(snapshot.InputsDigest); err != nil {
		return fmt.Errorf("renderer snapshot input digest is invalid")
	}
	if snapshot.Engine.Version == "" || len(snapshot.Artifacts) == 0 || len(snapshot.Artifacts) > 256 {
		return fmt.Errorf("renderer snapshot requires provenance and 1 to 256 artifacts")
	}
	seen := map[string]bool{}
	for _, artifact := range snapshot.Artifacts {
		name := artifact.Path
		allowed := name == "internal/mail/auth_delivery.go" || (path.Dir(name) == "internal/auth" && (strings.HasSuffix(name, ".go") || name == "internal/auth/README.md"))
		if !allowed || path.Clean(name) != name || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) || seen[name] || len(artifact.Source) > 4<<20 {
			return fmt.Errorf("invalid or duplicate auth artifact path or size: %q", name)
		}
		seen[name] = true
	}
	if !seen["internal/auth/service.go"] {
		return fmt.Errorf("auth snapshot requires internal/auth/service.go")
	}
	return nil
}

// Build compares exact output while preserving missing files and treating every supplied baseline as unverified provenance.
func Build(root string, target Snapshot, baseline *Snapshot) (Plan, error) {
	plan := Plan{Protocol: ProtocolVersion, Scope: "auth", Target: target.Engine, BaselineProvenance: "unknown", Files: []File{}, Notes: []string{
		"Experimental inspection only: no application files, dependencies, migrations, or generated output are changed.",
		"A supplied baseline is a comparison reference, not proof that this application adopted it. Both renderers use current inputs; historical input changes remain unknown.",
		"Declaration overlap is a review hint, not a semantic merge or proof of security compatibility. Inspect callers, persistence, frontend expectations, and regression tests.",
		"Scope: shared auth templates and neighboring auth files, plus selected derived-output context. Wiring, routes, migrations, frontend, and other framework surfaces require separate review.",
		"Derived output must be regenerated from actual application inputs after source reconciliation. This inspector does not judge whether derived bytes are correct.",
	}}
	if err := ValidateSnapshot(target); err != nil {
		return Plan{}, err
	}
	if baseline != nil {
		if err := ValidateSnapshot(*baseline); err != nil {
			return Plan{}, err
		}
		if baseline.InputsDigest != target.InputsDigest {
			return Plan{}, fmt.Errorf("renderer inputs differ; refusing to conflate configuration changes with template changes")
		}
		plan.Baseline = &baseline.Engine
		plan.BaselineProvenance = "supplied-unverified"
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, err
	}
	root, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return Plan{}, err
	}
	incoming, previous := artifactMap(target), map[string]string{}
	if baseline != nil {
		previous = artifactMap(*baseline)
	}
	paths := map[string]bool{}
	for name := range incoming {
		paths[name] = true
	}
	for name := range previous {
		paths[name] = true
	}
	for _, name := range sortedKeys(paths) {
		current, exists, err := readRegular(root, name)
		if err != nil {
			return Plan{}, err
		}
		next, hasNext := incoming[name]
		old, hasOld := previous[name]
		file := File{Path: name, Ownership: "framework-template"}
		if exists {
			file.CurrentDigest = Digest(current)
		}
		if hasNext {
			file.TargetDigest = Digest([]byte(next))
		}
		if hasOld {
			file.BaselineDigest = Digest([]byte(old))
		}
		switch {
		case !hasNext:
			file.Action = "review-retirement"
		case !exists:
			file.Action = "review-missing"
		case string(current) == next:
			file.Action = "matches-target"
		case baseline == nil:
			file.Action = "review-unversioned"
		case !hasOld:
			file.Action = "review-collision"
		case string(current) == old:
			file.Action = "upstream-only-relative-to-baseline"
		case next == old:
			file.Action = "preserve-local-relative-to-baseline"
		default:
			file.Action = "reconcile"
		}
		if exists && hasNext && strings.HasSuffix(name, ".go") {
			file.TargetDifferences, file.InvalidGo = changedDeclarations(string(current), next)
			if hasOld {
				var invalid bool
				file.LocalDifferences, invalid = changedDeclarations(old, string(current))
				file.InvalidGo = file.InvalidGo || invalid
				file.UpstreamDifferences, invalid = changedDeclarations(old, next)
				file.InvalidGo = file.InvalidGo || invalid
				for _, declaration := range file.LocalDifferences {
					if contains(file.UpstreamDifferences, declaration) {
						file.Overlaps = append(file.Overlaps, declaration)
					}
				}
			}
		}
		plan.Files = append(plan.Files, file)
	}
	// The catalog owns individual artifacts, never the directory containing owner additions.
	entries, err := os.ReadDir(filepath.Join(root, "internal/auth"))
	if err != nil && !os.IsNotExist(err) {
		return Plan{}, err
	}
	for _, entry := range entries {
		name := "internal/auth/" + entry.Name()
		if !paths[name] {
			plan.Files = append(plan.Files, File{Path: name, Ownership: "unknown-neighbor", Action: "preserve"})
		}
	}
	for _, name := range derivedContextPaths() {
		_, exists, err := readRegular(root, name)
		if err != nil {
			return Plan{}, err
		}
		if exists {
			plan.Files = append(plan.Files, File{Path: name, Ownership: "app-derived", Action: "regenerate-after-reconciliation"})
		}
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

// artifactMap retains explicit presence independently from empty source contents.
func artifactMap(snapshot Snapshot) map[string]string {
	result := make(map[string]string, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		result[artifact.Path] = artifact.Source
	}
	return result
}

// derivedContextPaths enumerates known generator contracts rather than guessing ownership from a filename suffix.
func derivedContextPaths() []string {
	paths := []string{"app/wire/wire_gen.go"}
	for _, resource := range []string{"caches", "queues", "storages"} {
		for _, output := range []string{"accessors_gen.go", "manager_gen.go"} {
			paths = append(paths, "internal/"+resource+"/"+output)
		}
	}
	return paths
}

// readRegular rejects symlinked ancestors and bounds reads without following application paths outside the project.
func readRegular(root, name string) ([]byte, bool, error) {
	parts := strings.Split(name, "/")
	current := root
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("inspect %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("inspect %s: symlinks are not supported", name)
		}
		if index != len(parts)-1 {
			if !info.IsDir() {
				return nil, false, fmt.Errorf("inspect %s: parent is not a directory", name)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > 4<<20 {
			return nil, false, fmt.Errorf("inspect %s: expected a regular file of at most 4 MiB", name)
		}
		data, err := os.ReadFile(current)
		return data, true, err
	}
	return nil, false, fmt.Errorf("empty artifact path")
}

// declarations fingerprints Go declarations while ignoring source positions and formatting differences.
func declarations(source string) (map[string]string, bool) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, "artifact.go", source, 0)
	if err != nil {
		return nil, true
	}
	result := map[string]string{}
	for _, declaration := range file.Decls {
		var key string
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			key = node.Name.Name
			if node.Recv != nil {
				var receiver bytes.Buffer
				_ = format.Node(&receiver, positions, node.Recv.List[0].Type)
				key = strings.TrimPrefix(receiver.String(), "*") + "." + key
			}
		case *ast.GenDecl:
			key = node.Tok.String()
			for _, spec := range node.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					key += " " + typed.Name.Name
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						key += " " + name.Name
					}
				}
			}
		}
		var output bytes.Buffer
		_ = format.Node(&output, positions, declaration)
		result[key] += output.String()
	}
	return result, false
}

// changedDeclarations highlights inspection targets without claiming that disjoint declarations are behaviorally independent.
func changedDeclarations(before, after string) ([]string, bool) {
	a, invalidA := declarations(before)
	b, invalidB := declarations(after)
	if invalidA || invalidB {
		return nil, true
	}
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	var changed []string
	for _, key := range sortedKeys(keys) {
		if a[key] != b[key] {
			changed = append(changed, key)
		}
	}
	return changed, false
}

// sortedKeys stabilizes plans independently of filesystem and map iteration order.
func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// contains checks declaration membership without changing the stable diagnostic order.
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
