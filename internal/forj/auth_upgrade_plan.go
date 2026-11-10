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
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

// authUpgradeConfig projects only non-secret template inputs so subprocess renderers never receive application files or owner configuration extras.
func authUpgradeConfig(root string) (*project.Config, error) {
	workspace, err := resolveProjectRenderWorkspace(root)
	if err != nil {
		return nil, err
	}
	if err := workspace.rejectEnvironmentSpecialFile(".goforj.yml"); err != nil {
		return nil, err
	}
	config, err := workspace.loadProjectConfig()
	if err != nil {
		return nil, err
	}
	components := appRenderComponents(config, project.DefaultApp())
	components.ResolveDependencies()
	if !components.Auth || !components.HasDatabase() || config.GoModuleName == "" {
		return nil, fmt.Errorf("experimental auth inspection requires a default Auth App, a database, and module_name")
	}
	return &project.Config{GoModuleName: config.GoModuleName, Render: project.RenderConfig{Components: components, StarterKit: appRenderStarterKit(config, project.DefaultApp())}}, nil
}

// authUpgradeSnapshot emits template bytes without invoking renderer mutations, resource discovery, or project tools.
func authUpgradeSnapshot(root string) (projectupgrade.Snapshot, error) {
	config, err := authUpgradeConfig(root)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	components := config.Render.Components
	data := struct {
		GoModuleName string             `json:"module"`
		Components   project.Components `json:"components"`
		StarterKit   project.StarterKit `json:"starter_kit"`
	}{config.GoModuleName, components, config.Render.StarterKit}
	inputs, err := json.Marshal(data)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	snapshot := projectupgrade.Snapshot{
		Protocol: projectupgrade.ProtocolVersion, Scope: "auth",
		Engine:       projectupgrade.Engine{Version: version.Version, Commit: version.Commit, Dirty: version.Dirty},
		InputsDigest: projectupgrade.Digest(inputs), Artifacts: []projectupgrade.Artifact{},
	}
	paths := authBaseTemplates()
	if components.OAuth {
		paths = append(paths, authOAuthTemplates()...)
	}
	sort.Strings(paths)
	for _, path := range paths {
		source, err := templatesFS.ReadFile(path)
		if err != nil {
			return projectupgrade.Snapshot{}, err
		}
		tmpl, err := parseProjectTemplate(path, source)
		if err != nil {
			return projectupgrade.Snapshot{}, err
		}
		var output bytes.Buffer
		if err := tmpl.Execute(&output, data); err != nil {
			return projectupgrade.Snapshot{}, err
		}
		destination := strings.TrimSuffix(path, ".tmpl")
		formatted, err := maybeFormatGoSource(destination, output.Bytes())
		if err != nil {
			return projectupgrade.Snapshot{}, err
		}
		snapshot.Artifacts = append(snapshot.Artifacts, projectupgrade.Artifact{Path: destination, Source: string(formatted)})
	}
	return snapshot, projectupgrade.ValidateSnapshot(snapshot)
}

// boundedSnapshotOutput prevents incompatible child renderers from producing unbounded protocol diagnostics.
type boundedSnapshotOutput struct{ buffer bytes.Buffer }

// Write bounds the complete snapshot rather than truncating JSON into an apparently valid response.
func (b *boundedSnapshotOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 16<<20 {
		return 0, fmt.Errorf("renderer snapshot exceeds 16 MiB")
	}
	return b.buffer.Write(data)
}

// externalAuthSnapshot lets one user-facing binary orchestrate another renderer without replacing global CLI installations.
func externalAuthSnapshot(root, binary string) (projectupgrade.Snapshot, error) {
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return projectupgrade.Snapshot{}, fmt.Errorf("locate comparison renderer: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	config, err := authUpgradeConfig(root)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	projection, err := yaml.Marshal(config)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	staging, err := os.MkdirTemp("", "goforj-auth-renderer-")
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	defer os.RemoveAll(staging)
	if err := os.WriteFile(filepath.Join(staging, ".goforj.yml"), projection, 0o600); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, resolved, "render", "--only", "auth", "--snapshot")
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
		return projectupgrade.Snapshot{}, fmt.Errorf("comparison renderer failed or lacks auth snapshot protocol v1: %w", err)
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

// runAuthUpgradeInspection keeps experimental plan flags outside the ordinary mutating render path.
func (c *RenderCmd) runAuthUpgradeInspection(output io.Writer) error {
	if c.Only != "auth" || len(c.Components) != 0 || c.Timings || c.Plan == c.Snapshot {
		return fmt.Errorf("auth inspection requires --only auth and exactly one of --plan or --snapshot; components and timings are unsupported")
	}
	if c.Snapshot && (c.FromRenderer != "" || c.TargetRenderer != "" || c.JSON) {
		return fmt.Errorf("--snapshot cannot be combined with renderer selection or --json")
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	target, err := authUpgradeSnapshot(root)
	if err != nil {
		return err
	}
	if c.TargetRenderer != "" {
		foreign, err := externalAuthSnapshot(root, c.TargetRenderer)
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
		previous, err := externalAuthSnapshot(root, c.FromRenderer)
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
	if _, err := fmt.Fprintln(output, "Experimental auth upgrade inspection (no changes applied)"); err != nil {
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

// authInspectionRequested rejects orphaned flags before any ordinary render can mutate a project.
func (c *RenderCmd) authInspectionRequested() (bool, error) {
	if !c.Plan && !c.Snapshot && (c.FromRenderer != "" || c.TargetRenderer != "" || c.JSON) {
		return false, fmt.Errorf("renderer selection and --json require --plan --only auth")
	}
	return c.Plan || c.Snapshot, nil
}

// runAuthUpgradeInspectionStdout preserves a separately testable writer boundary for machine-readable output.
func (c *RenderCmd) runAuthUpgradeInspectionStdout() error {
	return c.runAuthUpgradeInspection(os.Stdout)
}
