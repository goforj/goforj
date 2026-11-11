// Package projectupgrade inspects framework upgrades without publishing application changes.
package projectupgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"
)

// ProtocolVersion identifies the experimental renderer snapshot contract.
const ProtocolVersion = 3

// Snapshot contains reproducible scaffold output and ownership, never application or environment contents.
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

// Artifact describes one scaffold path or downstream generator contract without claiming ownership over its neighbors.
type Artifact struct {
	Path      string `json:"path"`
	Source    string `json:"source,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Ownership string `json:"ownership,omitempty"`
	Template  string `json:"template,omitempty"`
	Mode      uint32 `json:"mode,omitempty"`
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
	PreviousOwnership   string   `json:"previous_ownership,omitempty"`
	Action              string   `json:"action"`
	CurrentDigest       string   `json:"current_digest,omitempty"`
	BaselineDigest      string   `json:"baseline_digest,omitempty"`
	TargetDigest        string   `json:"target_digest,omitempty"`
	TargetDifferences   []string `json:"target_differences,omitempty"`
	LocalDifferences    []string `json:"local_differences,omitempty"`
	UpstreamDifferences []string `json:"upstream_differences,omitempty"`
	Overlaps            []string `json:"overlaps,omitempty"`
	InvalidGo           bool     `json:"invalid_go,omitempty"`
	ModeChanged         bool     `json:"mode_changed,omitempty"`
}

// Digest fingerprints exact output so reconstruction can be checked without storing baseline copies in an app.
func Digest(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

// validArtifactPath confines every renderer artifact to a relative project path without platform-specific escapes.
func validArtifactPath(name string) bool {
	if !fs.ValidPath(name) || name == "." || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.EqualFold(part, ".git") || strings.TrimRight(part, " .") != part {
			return false
		}
	}
	return true
}

// artifactOwnership returns only explicitly declared ownership so incomplete manifests cannot claim application source by default.
func artifactOwnership(artifact Artifact) string {
	return artifact.Ownership
}

// privateArtifact identifies environment layers without reading their values or fingerprints.
func privateArtifact(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(strings.ToLower(part), ".env") {
			return true
		}
	}
	return false
}

// ValidateSnapshot bounds a framework manifest and rejects source payloads for private, seeded, semantic, or derived artifacts.
func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.Protocol != ProtocolVersion || snapshot.Scope != "framework" {
		return fmt.Errorf("unsupported renderer snapshot protocol or scope")
	}
	if len(snapshot.InputsDigest) != 64 {
		return fmt.Errorf("renderer snapshot requires an input digest")
	}
	if _, err := hex.DecodeString(snapshot.InputsDigest); err != nil {
		return fmt.Errorf("renderer snapshot input digest is invalid")
	}
	if snapshot.Engine.Version == "" || !utf8.ValidString(snapshot.Engine.Version) || !utf8.ValidString(snapshot.Engine.Commit) || len(snapshot.Artifacts) == 0 || len(snapshot.Artifacts) > 16384 {
		return fmt.Errorf("renderer snapshot requires valid UTF-8 provenance and 1 to 16384 artifacts")
	}
	seen := map[string]bool{}
	portableNames := map[string]bool{}
	for _, artifact := range snapshot.Artifacts {
		name := artifact.Path
		if !validArtifactPath(name) || seen[name] || portableNames[strings.ToLower(name)] || len(artifact.Source) > 4<<20 || !utf8.ValidString(artifact.Source) || artifact.Mode > 0o777 {
			return fmt.Errorf("invalid or duplicate framework artifact path, size, encoding, or mode: %q", name)
		}
		if artifact.Template != "" && !validArtifactPath(artifact.Template) {
			return fmt.Errorf("invalid source template path")
		}
		switch artifactOwnership(artifact) {
		case "framework-template", "framework-asset", "migration":
			if privateArtifact(name) {
				return fmt.Errorf("environment artifacts must remain private")
			}
			if artifact.Ownership == "framework-asset" && artifact.Digest == "" {
				return fmt.Errorf("raw assets require an exact output digest")
			}
			if artifact.Ownership == "framework-asset" && artifact.Source != "" {
				return fmt.Errorf("raw assets must use digest-only payloads")
			}
			if artifact.Ownership != "framework-asset" && artifact.Digest != "" && Digest([]byte(artifact.Source)) != artifact.Digest {
				return fmt.Errorf("template source does not match its digest")
			}
		case "seed", "private", "semantic", "app-derived", "conditional-derived", "generated-context":
			if artifact.Source != "" || artifact.Digest != "" {
				return fmt.Errorf("non-managed artifacts must not contain source or digests")
			}
		default:
			return fmt.Errorf("unsupported artifact ownership")
		}
		if artifact.Digest != "" {
			if len(artifact.Digest) != 64 {
				return fmt.Errorf("invalid artifact digest")
			}
			if _, err := hex.DecodeString(artifact.Digest); err != nil {
				return fmt.Errorf("invalid artifact digest")
			}
			if artifact.Source != "" && Digest([]byte(artifact.Source)) != artifact.Digest {
				return fmt.Errorf("artifact source does not match its digest")
			}
		}
		seen[name] = true
		portableNames[strings.ToLower(name)] = true
	}
	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if portableNames[strings.ToLower(parent)] {
				return fmt.Errorf("artifact paths overlap as file and directory: %q", name)
			}
		}
	}
	return nil
}

// artifactDigest supports source-bearing templates and digest-only raw assets without storing binary baseline copies.
func artifactDigest(artifact Artifact) string {
	if artifact.Digest != "" {
		return artifact.Digest
	}
	return Digest([]byte(artifact.Source))
}

// Build classifies a framework-wide manifest without treating owner, private, semantic, or derived bytes as template drift.
func Build(root string, target Snapshot, baseline *Snapshot) (Plan, error) {
	plan := Plan{Protocol: ProtocolVersion, Scope: "framework", Target: target.Engine, BaselineProvenance: "unknown", Files: []File{}, Notes: []string{
		"Experimental framework inspection only: no application files, dependencies, migrations, or generated output are changed.",
		"A supplied baseline is a comparison reference, not proof that this application adopted it. Both renderers use current projected configuration; historical input changes remain unknown.",
		"Artifacts come from the ordinary renderer scaffold phase, including named Apps. Runtime environment and owner extension inputs are not copied; driver-dependent output uses component defaults and requires review.",
		"Seeded application files and neighboring owner additions are preserved. Environment layers are private; dependency and configuration files require semantic review.",
		"Declaration overlap is a review hint, not a semantic merge or proof of compatibility. App-derived output must be regenerated from actual reconciled application inputs; the planner does not judge its bytes.",
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
	filesystem, err := os.OpenRoot(absolute)
	if err != nil {
		return Plan{}, err
	}
	defer filesystem.Close()
	observations := map[string]observedFile{}
	incoming, previous := artifactMap(target), map[string]Artifact{}
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
		next, hasNext := incoming[name]
		old, hasOld := previous[name]
		artifact := next
		if !hasNext {
			artifact = old
		}
		ownership := artifactOwnership(artifact)
		file := File{Path: name, Ownership: ownership}
		if hasNext && hasOld && artifactOwnership(old) != ownership {
			file.PreviousOwnership = artifactOwnership(old)
			file.Action = "review-ownership-transition"
			plan.Files = append(plan.Files, file)
			continue
		}
		switch ownership {
		case "seed":
			file.Action = "preserve-seed"
		case "private":
			file.Action = "preserve-private"
		case "semantic":
			file.Action = "review-semantic"
		case "migration":
			file.Action = "preserve-migration"
			if _, err := filesystem.Lstat(filepath.FromSlash(name)); os.IsNotExist(err) {
				if !hasNext {
					continue
				}
				file.Action = "review-new-migration"
			} else if err != nil {
				return Plan{}, err
			} else if !hasNext {
				file.Action = "preserve-retired-migration"
			}
		case "app-derived":
			if _, err := filesystem.Lstat(filepath.FromSlash(name)); os.IsNotExist(err) {
				if !hasNext {
					continue
				}
				file.Action = "generate-derived-after-reconciliation"
			} else if err != nil {
				return Plan{}, err
			} else if !hasNext {
				file.Action = "review-retired-derived"
			} else {
				file.Action = "regenerate-after-reconciliation"
			}
		case "generated-context":
			file.Action = "review-generated-context"
		case "conditional-derived":
			file.Action = "review-derived-policy"
		default:
			current, observation, err := readManaged(filesystem, name, ownership != "framework-asset")
			if err != nil {
				return Plan{}, err
			}
			observations[name] = observation
			exists := observation.exists
			if exists {
				file.CurrentDigest = observation.digest
			}
			if hasNext {
				file.TargetDigest = artifactDigest(next)
			}
			if hasOld {
				file.BaselineDigest = artifactDigest(old)
			}
			if exists && next.Mode != 0 && runtime.GOOS != "windows" {
				file.ModeChanged = observation.mode&0o111 != next.Mode&0o111
			}

			matchesBaselineMode := runtime.GOOS == "windows" ||
				(old.Mode == 0 && !file.ModeChanged) ||
				(old.Mode != 0 && observation.mode&0o111 == old.Mode&0o111)
			unchangedUpstreamMode := runtime.GOOS == "windows" || next.Mode == 0 ||
				(old.Mode != 0 && old.Mode&0o111 == next.Mode&0o111)
			switch {
			case !hasNext:
				file.Action = "review-retirement"
			case !exists:
				file.Action = "review-missing"
			case file.CurrentDigest == file.TargetDigest && file.ModeChanged:
				file.Action = "review-mode"
			case file.CurrentDigest == file.TargetDigest:
				file.Action = "matches-target"
			case baseline == nil:
				file.Action = "review-unversioned"
			case !hasOld:
				file.Action = "review-collision"
			case file.CurrentDigest == file.BaselineDigest && matchesBaselineMode:
				file.Action = "upstream-only-relative-to-baseline"
			case file.TargetDigest == file.BaselineDigest && (!file.ModeChanged || unchangedUpstreamMode):
				file.Action = "preserve-local-relative-to-baseline"
			default:
				file.Action = "reconcile"
			}
			if exists && hasNext && ownership == "framework-template" && strings.HasSuffix(name, ".go") {
				file.TargetDifferences, file.InvalidGo = changedDeclarations(string(current), next.Source)
				if hasOld {
					var invalid bool
					file.LocalDifferences, invalid = changedDeclarations(old.Source, string(current))
					file.InvalidGo = file.InvalidGo || invalid
					file.UpstreamDifferences, invalid = changedDeclarations(old.Source, next.Source)
					file.InvalidGo = file.InvalidGo || invalid
					for _, declaration := range file.LocalDifferences {
						if contains(file.UpstreamDifferences, declaration) && contains(file.TargetDifferences, declaration) {
							file.Overlaps = append(file.Overlaps, declaration)
						}
					}
				}
			}
		}
		plan.Files = append(plan.Files, file)
	}
	prefixes := map[string]bool{}
	for name := range paths {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			prefixes[parent] = true
		}
	}
	if err := preserveUnknown(filesystem, "", paths, prefixes, &plan); err != nil {
		return Plan{}, err
	}
	if err := verifyObservations(filesystem, observations); err != nil {
		return Plan{}, err
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

// artifactMap retains explicit presence independently from empty source contents.
func artifactMap(snapshot Snapshot) map[string]Artifact {
	result := make(map[string]Artifact, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		result[artifact.Path] = artifact
	}
	return result
}

// declarations fingerprints Go declarations while ignoring source positions and formatting differences.
func declarations(source string) (map[string]string, bool) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, "artifact.go", source, 0)
	if err != nil {
		return nil, true
	}
	result := map[string]string{"package": file.Name.Name}
	var constraints, directives []string
	legacyEnd := legacyBuildConstraintEnd(source)
	var directiveOffsets []int
	var lexer scanner.Scanner
	lexer.Init(positions.AddFile("directives.go", -1, len(source)), []byte(source), nil, scanner.ScanComments)
	for {
		location, kind, line := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind != token.COMMENT {
			continue
		}
		if strings.HasPrefix(line, "//go:build ") {
			constraints = append(constraints, line)
		} else if positions.Position(location).Offset < legacyEnd && constraint.IsPlusBuild(line) {
			if _, err := constraint.Parse(line); err == nil {
				constraints = append(constraints, line)
			}
		}
		if strings.HasPrefix(line, "//go:") && !strings.HasPrefix(line, "//go:build ") {
			directives = append(directives, line)
			directiveOffsets = append(directiveOffsets, positions.Position(location).Offset)
		}
	}
	result["build-constraints"] = strings.Join(constraints, "\n")
	result["compiler-directives"] = strings.Join(directives, "\n")
	fragments := map[string][]string{}
	var initializationOrder []string
	directiveIndex := 0
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
			var names strings.Builder
			names.WriteString(node.Tok.String())
			for _, spec := range node.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					names.WriteByte(' ')
					names.WriteString(typed.Name.Name)
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						names.WriteByte(' ')
						names.WriteString(name.Name)
					}
				}
			}
			key = names.String()
			if node.Tok == token.VAR {
				initializationOrder = append(initializationOrder, key)
			}
		}
		// Bind directives to their next or enclosing declaration so relocation cannot disappear from the hints.
		for directiveIndex < len(directives) && directiveOffsets[directiveIndex] < positions.Position(declaration.End()).Offset {
			fragments[key] = append(fragments[key], directives[directiveIndex]+"\n")
			directiveIndex++
		}
		var output bytes.Buffer
		_ = format.Node(&output, positions, declaration)
		fragments[key] = append(fragments[key], output.String())
	}
	for key, parts := range fragments {
		result[key] = strings.Join(parts, "")
	}
	result["variable-initialization-order"] = strings.Join(initializationOrder, "\n")
	return result, false
}

// legacyBuildConstraintEnd preserves Go's blank-line boundary before the first non-line-comment header content.
func legacyBuildConstraintEnd(source string) int {
	end, offset := 0, 0
	if strings.HasPrefix(source, "\ufeff") {
		source = strings.TrimPrefix(source, "\ufeff")
		offset = len("\ufeff")
	}
	for len(source) > 0 {
		line, rest, hasNewline := strings.Cut(source, "\n")
		offset += len(line)
		if hasNewline {
			offset++
		}
		line = strings.TrimSpace(line)
		if line == "" {
			end = offset
		} else if !strings.HasPrefix(line, "//") {
			break
		}
		source = rest
	}
	return end
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
