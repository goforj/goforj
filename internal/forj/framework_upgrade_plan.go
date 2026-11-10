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

	"github.com/goforj/goforj/internal/projectupgrade"
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
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return projectupgrade.Snapshot{}, fmt.Errorf("locate comparison renderer: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	config, err := frameworkUpgradeConfig(root)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, resolved, "render", "--snapshot")
	command.Dir = staging
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "GOCACHE", "GOMODCACHE", "SYSTEMROOT", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	command.Env = append(command.Env, "APP_ENV=local")
	command.WaitDelay = time.Second
	var output boundedSnapshotOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return projectupgrade.Snapshot{}, fmt.Errorf("comparison renderer failed or lacks framework snapshot protocol v2: %w", err)
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
	target, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		return err
	}
	if c.TargetRenderer != "" {
		foreign, err := externalFrameworkSnapshot(root, c.TargetRenderer)
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
		return encoder.Encode(target)
	}
	var baseline *projectupgrade.Snapshot
	if c.FromRenderer != "" {
		previous, err := externalFrameworkSnapshot(root, c.FromRenderer)
		if err != nil {
			return err
		}
		baseline = &previous
	}
	plan, err := projectupgrade.Build(root, target, baseline)
	if err != nil {
		return err
	}
	if c.JSON {
		return encoder.Encode(plan)
	}
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
		if len(file.Overlaps) > 0 {
			if _, err := fmt.Fprintf(output, "  overlapping declarations: %s\n", strings.Join(file.Overlaps, ", ")); err != nil {
				return err
			}
		}
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
