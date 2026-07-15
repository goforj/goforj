package scenarios

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

//go:embed specs/*.yaml
var embeddedScenarioSpecs embed.FS

var scenarioIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// GenerateOptions controls scenario markdown generation.
type GenerateOptions struct {
	SpecDir string
	OutDir  string
	Check   bool
	All     bool
	IDs     []string
}

// ValidateOptions controls executable scenario validation.
type ValidateOptions struct {
	Logger   *logger.AppLogger
	SpecDir  string
	WorkDir  string
	Keep     bool
	All      bool
	IDs      []string
	ForjExec string
}

// scenarioCatalog keeps the validated graph and its lookup index together so execution cannot observe a different catalog than selection.
type scenarioCatalog struct {
	specs []ScenarioSpec
	byID  map[string]ScenarioSpec
}

// List returns known executable scenario specs.
func List(specDir string) ([]ScenarioSpec, error) {
	catalog, err := loadScenarioCatalog(specDir)
	if err != nil {
		return nil, err
	}
	return catalog.specs, nil
}

// Generate writes scenario markdown from executable specs.
func Generate(options GenerateOptions) error {
	catalog, err := loadScenarioCatalog(options.SpecDir)
	if err != nil {
		return err
	}
	specs, err := catalog.selectSpecs(options.IDs, options.All)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.OutDir) == "" {
		return fmt.Errorf("out dir is required")
	}
	for _, spec := range specs {
		body := []byte(renderScenarioMarkdown(spec))
		path, err := scenarioMarkdownPath(options.OutDir, spec.ID)
		if err != nil {
			return err
		}
		if options.Check {
			existing, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			if !bytes.Equal(existing, body) {
				return fmt.Errorf("generated markdown differs: %s", path)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		console.Successf("generated %s", path)
	}
	return nil
}

// Validate runs executable scenarios against rendered apps.
func Validate(options ValidateOptions) error {
	catalog, err := loadScenarioCatalog(options.SpecDir)
	if err != nil {
		return err
	}
	specs, err := catalog.selectSpecs(options.IDs, options.All)
	if err != nil {
		return err
	}
	forjExec := strings.TrimSpace(options.ForjExec)
	if forjExec == "" {
		var err error
		forjExec, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve current executable: %w", err)
		}
	}
	for _, spec := range specs {
		if err := runScenario(options, catalog, spec, forjExec); err != nil {
			return err
		}
	}
	return nil
}

// runScenario executes one selected scenario against the same validated catalog used to select it.
func runScenario(options ValidateOptions, catalog scenarioCatalog, spec ScenarioSpec, forjExec string) error {
	root, cleanup, err := scenarioRoot(options, spec)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	console.Actionf("scenario %s", spec.ID)
	if err := writeScenarioProjectConfig(root, spec); err != nil {
		return err
	}
	if err := runScenarioCommand(options.Logger, root, forjExec, ScenarioCommand{Run: []string{"forj", "render"}}, "render app"); err != nil {
		return err
	}
	if err := applyScenarioDependencies(options.Logger, root, forjExec, catalog.byID, spec, map[string]bool{}); err != nil {
		return err
	}
	for _, step := range spec.Steps {
		if err := applyScenarioStep(options.Logger, root, forjExec, spec, step); err != nil {
			return fmt.Errorf("%s step %q: %w", spec.ID, step.Title, err)
		}
	}
	for _, command := range spec.Verify.Commands {
		if err := runScenarioCommand(options.Logger, root, forjExec, command, "verify"); err != nil {
			return err
		}
	}
	console.Successf("scenario passed: %s", spec.ID)
	if options.Keep {
		console.Infof("scenario workdir: %s", root)
	}
	return nil
}

// scenarioRoot creates an isolated scenario directory only after the scenario ID is known to be path-safe.
func scenarioRoot(options ValidateOptions, spec ScenarioSpec) (string, func(), error) {
	if err := validateScenarioID(spec.ID); err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(options.WorkDir) != "" {
		root := filepath.Join(options.WorkDir, spec.ID)
		if err := os.RemoveAll(root); err != nil {
			return "", nil, err
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", nil, err
		}
		return root, nil, nil
	}
	root, err := os.MkdirTemp("", "forj-scenario-"+spec.ID+"-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		if !options.Keep {
			_ = os.RemoveAll(root)
		}
	}
	return root, cleanup, nil
}

// applyScenarioDependencies applies ancestors before their dependents so cumulative scenarios reproduce the documented golden path exactly once.
func applyScenarioDependencies(log *logger.AppLogger, root, forjExec string, specs map[string]ScenarioSpec, spec ScenarioSpec, applied map[string]bool) error {
	for _, depID := range spec.DependsOn {
		dep, ok := specs[depID]
		if !ok {
			return fmt.Errorf("%s depends on unknown scenario %q", spec.ID, depID)
		}
		if err := applyScenarioDependencies(log, root, forjExec, specs, dep, applied); err != nil {
			return err
		}
		if applied[dep.ID] {
			continue
		}
		for _, step := range dep.Steps {
			if err := applyScenarioStep(log, root, forjExec, dep, step); err != nil {
				return fmt.Errorf("%s dependency %s step %q: %w", spec.ID, dep.ID, step.Title, err)
			}
		}
		applied[dep.ID] = true
	}
	return nil
}

// ScenarioSpec is the executable and documentable contract for one verified scenario.
type ScenarioSpec struct {
	ID          string               `yaml:"id"`
	Title       string               `yaml:"title"`
	Description string               `yaml:"description"`
	DependsOn   []string             `yaml:"depends_on"`
	App         ScenarioApp          `yaml:"app"`
	Markdown    ScenarioMarkdown     `yaml:"markdown"`
	Steps       []ScenarioStep       `yaml:"steps"`
	Verify      ScenarioVerification `yaml:"verify"`
}

// ScenarioApp describes the rendered App that a scenario exercises.
type ScenarioApp struct {
	ModuleName    string             `yaml:"module_name"`
	DocModuleName string             `yaml:"doc_module_name"`
	Components    project.Components `yaml:"components"`
}

// ScenarioMarkdown carries narrative content generated from the same source as executable steps.
type ScenarioMarkdown struct {
	PathPosition      int               `yaml:"path_position"`
	PathTotal         int               `yaml:"path_total"`
	EstimatedMinutes  int               `yaml:"estimated_minutes"`
	Intro             string            `yaml:"intro"`
	WhatYouBuildTitle string            `yaml:"what_you_build_title"`
	WhatYouBuild      []string          `yaml:"what_you_build"`
	Diagrams          []Diagram         `yaml:"diagrams"`
	Prerequisites     string            `yaml:"prerequisites"`
	Before            string            `yaml:"before"`
	After             string            `yaml:"after"`
	Files             []string          `yaml:"files"`
	FileGroups        []FileGroup       `yaml:"file_groups"`
	GeneratedFiles    GeneratedFiles    `yaml:"generated_files"`
	ManualCheckTitle  string            `yaml:"manual_check_title"`
	ManualCheck       string            `yaml:"manual_check"`
	Sections          []MarkdownSection `yaml:"sections"`
	OperationsIntro   string            `yaml:"operations_intro"`
	Operations        []string          `yaml:"operations"`
	SwapDriver        string            `yaml:"swap_driver"`
	Troubleshooting   string            `yaml:"troubleshooting"`
	CommonMistakes    []string          `yaml:"common_mistakes"`
	NextSteps         []string          `yaml:"next_steps"`
}

// Diagram defines a fenced diagram included in generated scenario documentation.
type Diagram struct {
	Language string `yaml:"language"`
	Content  string `yaml:"content"`
}

// GeneratedFiles describes generator-owned files that readers should expect a scenario to change.
type GeneratedFiles struct {
	Intro string   `yaml:"intro"`
	Files []string `yaml:"files"`
	Note  string   `yaml:"note"`
}

// FileGroup keeps related scenario files together in generated documentation.
type FileGroup struct {
	Title string   `yaml:"title"`
	Files []string `yaml:"files"`
}

// MarkdownSection adds structured narrative that is not part of an executable step.
type MarkdownSection struct {
	Title   string `yaml:"title"`
	Content string `yaml:"content"`
}

// ScenarioStep combines the reader-facing explanation with one executable change or command.
type ScenarioStep struct {
	Title   string              `yaml:"title"`
	Explain string              `yaml:"explain"`
	Run     *ScenarioCommand    `yaml:"run,omitempty"`
	Write   *ScenarioFileChange `yaml:"write,omitempty"`
	Append  *ScenarioFileChange `yaml:"append,omitempty"`
	Replace *ScenarioReplace    `yaml:"replace,omitempty"`
}

// ScenarioFileChange defines content that a scenario writes or appends beneath its isolated root.
type ScenarioFileChange struct {
	Path     string `yaml:"path"`
	Language string `yaml:"language"`
	Content  string `yaml:"content"`
}

// ScenarioReplace defines an exact text transition so stale generated files fail loudly.
type ScenarioReplace struct {
	Path     string `yaml:"path"`
	Language string `yaml:"language"`
	Old      string `yaml:"old"`
	New      string `yaml:"new"`
}

// ScenarioVerification contains commands that prove the completed scenario remains executable.
type ScenarioVerification struct {
	Commands []ScenarioCommand `yaml:"commands"`
}

// ScenarioCommand defines a subprocess and the output fragments that demonstrate success.
type ScenarioCommand struct {
	Run      []string `yaml:"run"`
	Contains []string `yaml:"contains"`
}

// loadScenarioCatalog reads and validates the entire dependency graph before callers can mutate generated output or work directories.
func loadScenarioCatalog(specDir string) (scenarioCatalog, error) {
	specs, err := readScenarioSpecs(specDir)
	if err != nil {
		return scenarioCatalog{}, err
	}
	if err := validateScenarioCatalog(specs); err != nil {
		return scenarioCatalog{}, err
	}

	byID := make(map[string]ScenarioSpec, len(specs))
	for _, spec := range specs {
		byID[spec.ID] = spec
	}
	return scenarioCatalog{specs: specs, byID: byID}, nil
}

// loadScenarioSpecs preserves the package's list-oriented helper while guaranteeing callers receive a validated catalog.
func loadScenarioSpecs(specDir string) ([]ScenarioSpec, error) {
	catalog, err := loadScenarioCatalog(specDir)
	if err != nil {
		return nil, err
	}
	return catalog.specs, nil
}

// readScenarioSpecs decodes every source file before catalog-wide validation resolves graph relationships.
func readScenarioSpecs(specDir string) ([]ScenarioSpec, error) {
	var specs []ScenarioSpec
	if strings.TrimSpace(specDir) != "" {
		entries, err := os.ReadDir(specDir)
		if err != nil {
			return nil, fmt.Errorf("read spec dir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(specDir, entry.Name()))
			if err != nil {
				return nil, err
			}
			spec, err := decodeScenarioSpec(body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			specs = append(specs, spec)
		}
	} else {
		entries, err := embeddedScenarioSpecs.ReadDir("specs")
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			body, err := embeddedScenarioSpecs.ReadFile("specs/" + entry.Name())
			if err != nil {
				return nil, err
			}
			spec, err := decodeScenarioSpec(body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			specs = append(specs, spec)
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs, nil
}

// decodeScenarioSpec applies per-file defaults while catalog-wide constraints remain centralized in validateScenarioCatalog.
func decodeScenarioSpec(body []byte) (ScenarioSpec, error) {
	var spec ScenarioSpec
	if err := yaml.Unmarshal(body, &spec); err != nil {
		return spec, err
	}
	if strings.TrimSpace(spec.ID) == "" {
		return spec, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(spec.Title) == "" {
		return spec, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(spec.App.ModuleName) == "" {
		spec.App.ModuleName = "example.com/" + strings.ReplaceAll(spec.ID, "-", "")
	}
	return spec, nil
}

// selectedScenarioSpecs preserves the package's selection helper for tests and callers that do not need graph lookups.
func selectedScenarioSpecs(specDir string, ids []string, all bool) ([]ScenarioSpec, error) {
	catalog, err := loadScenarioCatalog(specDir)
	if err != nil {
		return nil, err
	}
	return catalog.selectSpecs(ids, all)
}

// selectSpecs resolves requested IDs only from the validated catalog index.
func (catalog scenarioCatalog) selectSpecs(ids []string, all bool) ([]ScenarioSpec, error) {
	if all || len(ids) == 0 {
		return catalog.specs, nil
	}
	selected := make([]ScenarioSpec, 0, len(ids))
	for _, id := range ids {
		spec, ok := catalog.byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q", id)
		}
		selected = append(selected, spec)
	}
	return selected, nil
}

// validateScenarioCatalog rejects IDs and dependency graphs that could make execution unsafe or ambiguous.
func validateScenarioCatalog(specs []ScenarioSpec) error {
	byID := make(map[string]ScenarioSpec, len(specs))
	for _, spec := range specs {
		if err := validateScenarioID(spec.ID); err != nil {
			return err
		}
		if _, exists := byID[spec.ID]; exists {
			return fmt.Errorf("duplicate scenario ID %q", spec.ID)
		}
		byID[spec.ID] = spec
	}

	for _, spec := range specs {
		for _, dependencyID := range spec.DependsOn {
			if err := validateScenarioID(dependencyID); err != nil {
				return fmt.Errorf("scenario %q has invalid dependency: %w", spec.ID, err)
			}
			if _, exists := byID[dependencyID]; !exists {
				return fmt.Errorf("scenario %q depends on unknown scenario %q", spec.ID, dependencyID)
			}
		}
	}

	visiting := make(map[string]bool, len(specs))
	visited := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if err := validateScenarioDependencyGraph(spec, byID, visiting, visited, nil); err != nil {
			return err
		}
	}
	return nil
}

// validateScenarioDependencyGraph uses visiting and visited sets so shared dependencies are accepted while back edges report their complete cycle.
func validateScenarioDependencyGraph(spec ScenarioSpec, byID map[string]ScenarioSpec, visiting, visited map[string]bool, path []string) error {
	if visited[spec.ID] {
		return nil
	}
	if visiting[spec.ID] {
		cycleStart := 0
		for i, id := range path {
			if id == spec.ID {
				cycleStart = i
				break
			}
		}
		cycle := append(append([]string{}, path[cycleStart:]...), spec.ID)
		return fmt.Errorf("scenario dependency cycle: %s", strings.Join(cycle, " -> "))
	}

	visiting[spec.ID] = true
	path = append(path, spec.ID)
	for _, dependencyID := range spec.DependsOn {
		if err := validateScenarioDependencyGraph(byID[dependencyID], byID, visiting, visited, path); err != nil {
			return err
		}
	}
	delete(visiting, spec.ID)
	visited[spec.ID] = true
	return nil
}

// validateScenarioID enforces a platform-independent slug because scenario IDs become directory and file names.
func validateScenarioID(id string) error {
	if !scenarioIDPattern.MatchString(id) {
		return fmt.Errorf("scenario ID %q must be a safe slug using lowercase letters, numbers, and single hyphens", id)
	}
	return nil
}

// scenarioMarkdownPath validates the filename component before joining it to user-selected output.
func scenarioMarkdownPath(outDir, id string) (string, error) {
	if err := validateScenarioID(id); err != nil {
		return "", err
	}
	return filepath.Join(outDir, id+".md"), nil
}

// writeScenarioProjectConfig marks scenario selections as current so intentionally absent primitives stay disabled.
func writeScenarioProjectConfig(root string, spec ScenarioSpec) error {
	cfg := project.Config{
		ProjectName:  spec.Title,
		GoModuleName: spec.App.ModuleName,
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			Components:               spec.App.Components,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
		Dev: project.DevConfig{
			Pre:               []project.DevTask{},
			Down:              []project.DevTask{},
			DownOnExit:        false,
			SoundOnWatchError: false,
			Watches:           []project.DevWatch{},
			WirePaths:         []string{project.DefaultApp().WireDir},
		},
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".goforj.yml"), body, 0o644)
}

// applyScenarioStep preserves the declared action order because later scenario edits can depend on earlier generated content.
func applyScenarioStep(log *logger.AppLogger, root, forjExec string, spec ScenarioSpec, step ScenarioStep) error {
	if step.Write != nil {
		if err := writeScenarioFile(root, spec, *step.Write); err != nil {
			return err
		}
	}
	if step.Append != nil {
		if err := appendScenarioFile(root, spec, *step.Append); err != nil {
			return err
		}
	}
	if step.Replace != nil {
		if err := replaceScenarioText(root, spec, *step.Replace); err != nil {
			return err
		}
	}
	if step.Run != nil {
		if err := runScenarioCommand(log, root, forjExec, *step.Run, step.Title); err != nil {
			return err
		}
	}
	return nil
}

// writeScenarioFile formats valid Go sources so executable examples remain stable across generated documentation and tests.
func writeScenarioFile(root string, spec ScenarioSpec, change ScenarioFileChange) error {
	path, err := scenarioPath(root, change.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := []byte(expandScenarioText(spec, change.Content))
	if strings.HasSuffix(path, ".go") {
		if formatted, err := format.Source(content); err == nil {
			content = formatted
		}
	}
	return os.WriteFile(path, content, 0o644)
}

// appendScenarioFile preserves existing generated content for scenario steps that intentionally extend a file.
func appendScenarioFile(root string, spec ScenarioSpec, change ScenarioFileChange) error {
	path, err := scenarioPath(root, change.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := []byte(expandScenarioText(spec, change.Content))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return err
	}
	return nil
}

// replaceScenarioText requires an exact old value so template drift cannot silently produce an incomplete scenario.
func replaceScenarioText(root string, spec ScenarioSpec, change ScenarioReplace) error {
	path, err := scenarioPath(root, change.Path)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	oldText := expandScenarioText(spec, change.Old)
	newText := expandScenarioText(spec, change.New)
	updated := strings.Replace(string(body), oldText, newText, 1)
	if updated == string(body) {
		return fmt.Errorf("replace target not found in %s", change.Path)
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

// scenarioPath confines every declared file change to the isolated scenario root.
func scenarioPath(root, rel string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(rel))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid scenario path %q", rel)
	}
	return filepath.Join(root, clean), nil
}

// runScenarioCommand substitutes the current Forj executable so scenarios validate the build under test rather than an installed release.
func runScenarioCommand(log *logger.AppLogger, root, forjExec string, command ScenarioCommand, label string) error {
	if len(command.Run) == 0 {
		return fmt.Errorf("command is required")
	}
	args := append([]string{}, command.Run...)
	if args[0] == "forj" {
		args[0] = forjExec
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	cmd.Env = scenarioProcessEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if text == "" {
			text = err.Error()
		}
		log.Error().Str("step", label).Str("command", strings.Join(command.Run, " ")).Str("output", text).Msg("scenario command failed")
		return fmt.Errorf("%s: %w\n%s", strings.Join(command.Run, " "), err, text)
	}
	output := out.String()
	for _, expected := range command.Contains {
		if !strings.Contains(output, expected) {
			return fmt.Errorf("%s: output missing %q\n%s", strings.Join(command.Run, " "), expected, output)
		}
	}
	return nil
}

// scenarioProcessEnv isolates Go caches so scenario subprocesses do not depend on a developer's global cache state.
func scenarioProcessEnv() []string {
	modCache, buildCache := testkit.GoCachePaths()
	return testkit.ProcessGoEnv("", map[string]string{
		"GOMODCACHE": modCache,
		"GOCACHE":    buildCache,
	})
}

// expandScenarioText resolves the executable module independently from the documentation-safe module name.
func expandScenarioText(spec ScenarioSpec, value string) string {
	value = strings.ReplaceAll(value, "{{module}}", spec.App.ModuleName)
	return value
}

// expandScenarioMarkdownText prefers the documentation module so published examples can remain stable across executable fixtures.
func expandScenarioMarkdownText(spec ScenarioSpec, value string) string {
	module := strings.TrimSpace(spec.App.DocModuleName)
	if module == "" {
		module = spec.App.ModuleName
	}
	value = strings.ReplaceAll(value, "{{module}}", module)
	return value
}

// renderScenarioMarkdown keeps published guidance derived from the same ordered steps exercised by validation.
func renderScenarioMarkdown(spec ScenarioSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: %s\ndescription: %s\n---\n\n", spec.Title, spec.Description)
	fmt.Fprintf(&b, "# %s\n\n", spec.Title)
	b.WriteString("::: info Verified Scenario\n")
	b.WriteString("This page is generated from an executable spec. An automated suite renders a fresh App from the current GoForj templates, applies every step below in order, and runs every verification command. If any step fails, the page does not ship.\n")
	b.WriteString(":::\n\n")
	if spec.Markdown.PathPosition > 0 && spec.Markdown.PathTotal > 0 {
		fmt.Fprintf(&b, "Scenario %d of %d in the [verified path](/scenarios/).", spec.Markdown.PathPosition, spec.Markdown.PathTotal)
		if spec.Markdown.EstimatedMinutes > 0 {
			fmt.Fprintf(&b, " Plan on about %d minutes.", spec.Markdown.EstimatedMinutes)
		}
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(spec.Markdown.Intro) != "" {
		b.WriteString(strings.TrimSpace(spec.Markdown.Intro))
		b.WriteString("\n\n")
	}
	if len(spec.Markdown.WhatYouBuild) > 0 {
		title := strings.TrimSpace(spec.Markdown.WhatYouBuildTitle)
		if title == "" {
			title = "What You Will Build"
		}
		fmt.Fprintf(&b, "## %s\n\n", title)
		for _, item := range spec.Markdown.WhatYouBuild {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}
	for _, diagram := range spec.Markdown.Diagrams {
		if strings.TrimSpace(diagram.Content) == "" {
			continue
		}
		language := strings.TrimSpace(diagram.Language)
		if language == "" {
			language = "mermaid"
		}
		writeCodeBlock(&b, language, diagram.Content, false)
	}
	if strings.TrimSpace(spec.Markdown.Prerequisites) != "" {
		b.WriteString("## Prerequisites\n\n")
		b.WriteString(strings.TrimSpace(spec.Markdown.Prerequisites))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(spec.Markdown.Before) != "" || strings.TrimSpace(spec.Markdown.After) != "" {
		b.WriteString("## Golden Path State\n\n")
		if strings.TrimSpace(spec.Markdown.Before) != "" {
			b.WriteString("Before this scenario, ")
			b.WriteString(strings.TrimSpace(spec.Markdown.Before))
			b.WriteString("\n\n")
		}
		if strings.TrimSpace(spec.Markdown.After) != "" {
			b.WriteString("After this scenario, ")
			b.WriteString(strings.TrimSpace(spec.Markdown.After))
			b.WriteString("\n\n")
		}
	}
	if len(spec.Markdown.FileGroups) > 0 || len(spec.Markdown.Files) > 0 {
		b.WriteString("## Files\n\n")
		b.WriteString("This scenario edits or creates:\n\n")
		if len(spec.Markdown.FileGroups) > 0 {
			for _, group := range spec.Markdown.FileGroups {
				if len(group.Files) == 0 {
					continue
				}
				title := strings.TrimSpace(group.Title)
				if title != "" {
					fmt.Fprintf(&b, "**%s**\n\n", title)
				}
				b.WriteString("```text\n")
				for _, file := range group.Files {
					b.WriteString(file)
					b.WriteString("\n")
				}
				b.WriteString("```\n\n")
			}
		} else {
			b.WriteString("```text\n")
			for _, file := range spec.Markdown.Files {
				b.WriteString(file)
				b.WriteString("\n")
			}
			b.WriteString("```\n\n")
		}
	}
	if len(spec.Markdown.GeneratedFiles.Files) > 0 {
		if strings.TrimSpace(spec.Markdown.GeneratedFiles.Intro) != "" {
			b.WriteString(strings.TrimSpace(spec.Markdown.GeneratedFiles.Intro))
		} else {
			b.WriteString("The generator updates:")
		}
		b.WriteString("\n\n```text\n")
		for _, file := range spec.Markdown.GeneratedFiles.Files {
			b.WriteString(file)
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
		if strings.TrimSpace(spec.Markdown.GeneratedFiles.Note) != "" {
			b.WriteString(strings.TrimSpace(spec.Markdown.GeneratedFiles.Note))
			b.WriteString("\n\n")
		}
	}
	for i, step := range spec.Steps {
		fmt.Fprintf(&b, "## Step %d: %s\n\n", i+1, step.Title)
		if strings.TrimSpace(step.Explain) != "" {
			b.WriteString(strings.TrimSpace(step.Explain))
			b.WriteString("\n\n")
		}
		if step.Write != nil {
			fmt.Fprintf(&b, "Create or replace `%s`:\n\n", step.Write.Path)
			writeCodeBlock(&b, step.Write.Language, expandScenarioMarkdownText(spec, step.Write.Content), true)
		}
		if step.Append != nil {
			fmt.Fprintf(&b, "Append to `%s`:\n\n", step.Append.Path)
			writeCodeBlock(&b, step.Append.Language, expandScenarioMarkdownText(spec, step.Append.Content), false)
		}
		if step.Replace != nil {
			newContent := expandScenarioMarkdownText(spec, step.Replace.New)
			if strings.TrimSpace(newContent) == "" {
				fmt.Fprintf(&b, "Remove from `%s`:\n\n", step.Replace.Path)
				writeCodeBlock(&b, step.Replace.Language, expandScenarioMarkdownText(spec, step.Replace.Old), false)
			} else {
				fmt.Fprintf(&b, "Update `%s` so it includes:\n\n", step.Replace.Path)
				writeCodeBlock(&b, step.Replace.Language, newContent, false)
			}
		}
		if step.Run != nil && len(step.Run.Run) > 0 {
			b.WriteString("```bash\n")
			b.WriteString(strings.Join(step.Run.Run, " "))
			b.WriteString("\n```\n\n")
		}
	}
	if len(spec.Verify.Commands) > 0 {
		b.WriteString("## Build and Verify\n\n")
		for _, command := range spec.Verify.Commands {
			if len(command.Run) == 0 {
				continue
			}
			b.WriteString("```bash\n")
			b.WriteString(strings.Join(command.Run, " "))
			b.WriteString("\n```\n\n")
			if len(command.Contains) > 0 {
				b.WriteString("Expected output includes:\n\n")
				for _, item := range command.Contains {
					fmt.Fprintf(&b, "- `%s`\n", item)
				}
				b.WriteString("\n")
			}
		}
	}
	if strings.TrimSpace(spec.Markdown.ManualCheck) != "" {
		title := strings.TrimSpace(spec.Markdown.ManualCheckTitle)
		if title == "" {
			title = "Try The Route"
		}
		fmt.Fprintf(&b, "## %s\n\n", title)
		b.WriteString(strings.TrimSpace(spec.Markdown.ManualCheck))
		b.WriteString("\n\n")
	}
	for _, section := range spec.Markdown.Sections {
		if strings.TrimSpace(section.Title) == "" || strings.TrimSpace(section.Content) == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", strings.TrimSpace(section.Title))
		b.WriteString(strings.TrimSpace(section.Content))
		b.WriteString("\n\n")
	}
	if len(spec.Markdown.Operations) > 0 {
		b.WriteString("## Operations\n\n")
		intro := strings.TrimSpace(spec.Markdown.OperationsIntro)
		if intro == "" {
			intro = "Operational notes:"
		}
		b.WriteString(intro)
		b.WriteString("\n\n")
		for _, item := range spec.Markdown.Operations {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(spec.Markdown.SwapDriver) != "" {
		b.WriteString("## Swap The Driver\n\n")
		b.WriteString(strings.TrimSpace(spec.Markdown.SwapDriver))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(spec.Markdown.Troubleshooting) != "" {
		b.WriteString("## Troubleshooting\n\n")
		b.WriteString(strings.TrimSpace(spec.Markdown.Troubleshooting))
		b.WriteString("\n\n")
	}
	if len(spec.Markdown.CommonMistakes) > 0 {
		b.WriteString("## Common Mistakes\n\n::: warning Common mistakes\n")
		for _, item := range spec.Markdown.CommonMistakes {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString(":::\n\n")
	}
	if len(spec.Markdown.NextSteps) > 0 {
		b.WriteString("## Next Steps\n\n")
		for _, item := range spec.Markdown.NextSteps {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// writeCodeBlock normalizes fenced content so generated Markdown remains deterministic.
func writeCodeBlock(b *strings.Builder, language, content string, formatContent bool) {
	language = strings.TrimSpace(language)
	content = strings.Trim(content, "\n")
	if formatContent && language == "go" {
		if formatted, err := format.Source([]byte(content)); err == nil {
			content = string(formatted)
		}
	}
	fmt.Fprintf(b, "```%s\n", language)
	b.WriteString(strings.TrimRight(content, "\n"))
	b.WriteString("\n```\n\n")
}
