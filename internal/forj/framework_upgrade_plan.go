package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
)

// boundedSnapshotOutput prevents incompatible child renderers from producing unbounded protocol diagnostics.
type boundedSnapshotOutput struct{ buffer bytes.Buffer }

// Write bounds the complete snapshot rather than truncating JSON into an apparently valid response.
func (b *boundedSnapshotOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 32<<20 {
		return 0, fmt.Errorf("renderer snapshot exceeds 32 MiB")
	}
	return b.buffer.Write(data)
}

// externalFrameworkSnapshot lets one user-facing binary orchestrate another renderer without replacing global CLI installations.
func externalFrameworkSnapshot(root, binary string) (projectupgrade.Snapshot, error) {
	config, err := frameworkUpgradeConfig(root)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	return frameworkSnapshotFromRenderer(context.Background(), config, binary)
}

// frameworkSnapshotFromRenderer isolates a renderer while preserving a caller's cancellation and frozen input recipe.
func frameworkSnapshotFromRenderer(ctx context.Context, config *project.Config, binary string) (projectupgrade.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return projectupgrade.Snapshot{}, fmt.Errorf("comparison renderer interrupted: %w", err)
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return projectupgrade.Snapshot{}, fmt.Errorf("locate comparison renderer: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	staging, err := os.MkdirTemp("", "goforj-framework-renderer-")
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	defer os.RemoveAll(staging)
	if err := writeFrameworkProjection(staging, config); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A killed peer cannot run deferred cleanup, so its temporary trees belong to this workspace.
	childTemporary := filepath.Join(staging, "renderer-tmp")
	if err := os.Mkdir(childTemporary, 0o700); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	environment := map[string]string{"APP_ENV": "local", "TMPDIR": childTemporary, "TMP": childTemporary, "TEMP": childTemporary}
	for _, key := range []string{"PATH", "HOME", "GOCACHE", "GOMODCACHE", "SYSTEMROOT", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			environment[key] = value
		}
	}
	var output boundedSnapshotOutput
	supervisor := devwatch.NewSupervisor(devwatch.SupervisorOptions{StopTimeout: 100 * time.Millisecond})
	defer supervisor.Close()
	_, runErr := supervisor.Run(ctx, "comparison-renderer", devwatch.Command{
		Args: []string{resolved, "render", "--snapshot"}, Dir: staging,
		Env: environment, ReplaceEnv: true, Stdout: &output, Stderr: io.Discard,
	})
	if runErr != nil {
		if ctx.Err() != nil {
			return projectupgrade.Snapshot{}, fmt.Errorf("comparison renderer interrupted: %w", ctx.Err())
		}
		return projectupgrade.Snapshot{}, fmt.Errorf("comparison renderer failed or lacks framework snapshot protocol v%d: %w", projectupgrade.ProtocolVersion, runErr)
	}
	if err := validateSnapshotJSON(output.buffer.Bytes()); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(output.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var snapshot projectupgrade.Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, fmt.Errorf("decode comparison renderer snapshot: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return snapshot, fmt.Errorf("comparison renderer returned trailing data")
	}
	return snapshot, projectupgrade.ValidateSnapshot(snapshot)
}

// validateSnapshotJSON rejects ambiguous duplicate fields, excessive nesting, and extra documents before typed protocol decoding.
func validateSnapshotJSON(source []byte) error {
	if !utf8.Valid(source) {
		return fmt.Errorf("renderer snapshot JSON must use valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	if err := consumeSnapshotJSONValue(decoder, 0); err != nil {
		return fmt.Errorf("invalid renderer snapshot JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("renderer snapshot must contain exactly one JSON document")
	}
	return nil
}

// consumeSnapshotJSONValue bounds metadata structure while treating generated source strings as opaque payloads.
func consumeSnapshotJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("snapshot nesting exceeds 32 levels")
	}
	value, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, container := value.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("invalid snapshot object key")
			}
			for _, character := range name {
				if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
					return fmt.Errorf("snapshot fields require canonical lowercase ASCII names")
				}
			}
			if seen[name] {
				return fmt.Errorf("duplicate snapshot object field")
			}
			seen[name] = true
			if err := consumeSnapshotJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := consumeSnapshotJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected snapshot delimiter")
	}
	_, err = decoder.Token()
	return err
}

// runFrameworkUpgradeInspection keeps experimental plan flags outside the ordinary mutating render path.
func (c *RenderCmd) runFrameworkUpgradeInspection(output io.Writer) error {
	if c.Only != "" || len(c.Components) != 0 || c.Timings || c.Plan == c.Snapshot {
		return fmt.Errorf("framework inspection requires exactly one of --plan or --snapshot; --only, components, and timings are unsupported")
	}
	if c.Snapshot && (c.FromRenderer != "" || c.TargetRenderer != "" || c.JSON) {
		return fmt.Errorf("--snapshot cannot be combined with renderer selection or --json")
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	config, err := frameworkUpgradeConfig(root)
	if err != nil {
		return err
	}
	target, err := frameworkSnapshotForConfig(config)
	if err != nil {
		return err
	}
	if c.TargetRenderer != "" {
		foreign, err := frameworkSnapshotFromRenderer(context.Background(), config, c.TargetRenderer)
		if err != nil {
			return err
		}
		if foreign.InputsDigest != target.InputsDigest {
			return fmt.Errorf("target renderer uses different inputs or an incompatible input contract")
		}
		target = foreign
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if c.Snapshot {
		if err := verifyFrameworkInputs(root, target.InputsDigest); err != nil {
			return err
		}
		return encoder.Encode(target)
	}
	var baseline *projectupgrade.Snapshot
	if c.FromRenderer != "" {
		previous, err := frameworkSnapshotFromRenderer(context.Background(), config, c.FromRenderer)
		if err != nil {
			return err
		}
		baseline = &previous
	}
	plan, err := projectupgrade.Build(root, target, baseline)
	if err != nil {
		return err
	}
	if err := verifyFrameworkInputs(root, target.InputsDigest); err != nil {
		return err
	}
	if c.JSON {
		return encoder.Encode(plan)
	}
	return writeFrameworkUpgradePlan(output, plan)
}

// writeFrameworkUpgradePlan exposes material syntax and mode diagnostics alongside review actions for human readers.
func writeFrameworkUpgradePlan(output io.Writer, plan projectupgrade.Plan) error {
	if _, err := fmt.Fprintln(output, "Experimental framework upgrade inspection (no changes applied)"); err != nil {
		return err
	}
	for _, note := range plan.Notes {
		if _, err := fmt.Fprintln(output, note); err != nil {
			return err
		}
	}
	for _, file := range plan.Files {
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\n", file.Path, file.Ownership, file.Action); err != nil {
			return err
		}
		if file.InvalidGo {
			if _, err := fmt.Fprintln(output, "  Go declaration hints unavailable: invalid Go syntax in a comparison input"); err != nil {
				return err
			}
		}
		if file.ModeChanged {
			if _, err := fmt.Fprintln(output, "  executable bits differ from target"); err != nil {
				return err
			}
		}
		if len(file.Overlaps) > 0 {
			if _, err := fmt.Fprintf(output, "  overlapping declarations: %s\n", strings.Join(file.Overlaps, ", ")); err != nil {
				return err
			}
		}
	}
	return nil
}

// verifyFrameworkInputs rejects configuration or App-discovery changes before emitting a plan based on older intent.
func verifyFrameworkInputs(root, expected string) error {
	config, err := frameworkUpgradeConfig(root)
	if err != nil {
		return err
	}
	digest, err := frameworkInputsDigest(config)
	if err != nil {
		return err
	}
	if digest != expected {
		return fmt.Errorf("render inputs changed during planning; retry the inspection")
	}
	return nil
}

// frameworkInspectionRequested rejects orphaned flags before any ordinary render can mutate a project.
func (c *RenderCmd) frameworkInspectionRequested() (bool, error) {
	if !c.Plan && !c.Snapshot && (c.FromRenderer != "" || c.TargetRenderer != "" || c.JSON) {
		return false, fmt.Errorf("renderer selection and --json require --plan")
	}
	return c.Plan || c.Snapshot, nil
}

// runFrameworkUpgradeInspectionStdout preserves a separately testable writer boundary for machine-readable output.
func (c *RenderCmd) runFrameworkUpgradeInspectionStdout() error {
	return c.runFrameworkUpgradeInspection(os.Stdout)
}
