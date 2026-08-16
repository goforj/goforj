package scenarios

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

const liveScenarioSchemaVersion = 2

// scenarioSpecV1 is the frozen legacy wire contract used by existing and external catalogs.
type scenarioSpecV1 struct {
	ID          string               `yaml:"id"`
	Title       string               `yaml:"title"`
	Description string               `yaml:"description"`
	DependsOn   []string             `yaml:"depends_on"`
	App         ScenarioApp          `yaml:"app"`
	Markdown    ScenarioMarkdown     `yaml:"markdown"`
	Steps       []ScenarioStep       `yaml:"steps"`
	Verify      ScenarioVerification `yaml:"verify"`
}

// scenarioSpecV2 is the live-evaluable wire contract with an explicit preparation boundary.
type scenarioSpecV2 struct {
	SchemaVersion int                   `yaml:"schema_version"`
	ID            string                `yaml:"id"`
	Title         string                `yaml:"title"`
	Description   string                `yaml:"description"`
	DependsOn     []string              `yaml:"depends_on"`
	App           ScenarioApp           `yaml:"app"`
	Markdown      ScenarioMarkdown      `yaml:"markdown"`
	Prepare       scenarioPreparationV2 `yaml:"prepare"`
	Steps         []scenarioStepV2      `yaml:"steps"`
	Checks        []scenarioCheckV2     `yaml:"checks"`
}

// scenarioPreparationV2 keeps fixture actions separate from the target an agent must implement.
type scenarioPreparationV2 struct {
	Steps  []scenarioStepV2  `yaml:"steps"`
	Checks []scenarioCheckV2 `yaml:"checks"`
}

// scenarioStepV2 is a closed action union; semantic validation requires exactly one action.
type scenarioStepV2 struct {
	ID      string              `yaml:"id"`
	Title   string              `yaml:"title"`
	Explain string              `yaml:"explain"`
	Command []string            `yaml:"command,omitempty"`
	Write   *ScenarioFileChange `yaml:"write,omitempty"`
	Append  *ScenarioFileChange `yaml:"append,omitempty"`
	Replace *ScenarioReplace    `yaml:"replace,omitempty"`
}

// scenarioCheckV2 declares one structured command and optional output evidence.
type scenarioCheckV2 struct {
	Command  []string `yaml:"command"`
	Contains []string `yaml:"contains"`
}

// decodeScenarioSpec selects a strict schema before applying defaults and semantic validation.
func decodeScenarioSpec(body []byte) (ScenarioSpec, error) {
	document, version, err := decodeScenarioDocument(body)
	if err != nil {
		return ScenarioSpec{}, err
	}

	var spec ScenarioSpec
	switch version {
	case 0:
		var wire scenarioSpecV1
		if err := decodeKnownScenarioFields(body, &wire); err != nil {
			return ScenarioSpec{}, err
		}
		spec = normalizeScenarioV1(wire)
	case liveScenarioSchemaVersion:
		if err := rejectScenarioYAMLFeatures(document); err != nil {
			return ScenarioSpec{}, err
		}
		var wire scenarioSpecV2
		if err := decodeKnownScenarioFields(body, &wire); err != nil {
			return ScenarioSpec{}, err
		}
		var normalizeErr error
		spec, normalizeErr = normalizeScenarioV2(wire)
		if normalizeErr != nil {
			return ScenarioSpec{}, normalizeErr
		}
	default:
		return ScenarioSpec{}, fmt.Errorf("unsupported schema_version %d", version)
	}

	if strings.TrimSpace(spec.ID) == "" {
		return ScenarioSpec{}, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(spec.Title) == "" {
		return ScenarioSpec{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(spec.App.ModuleName) == "" {
		spec.App.ModuleName = "example.com/" + strings.ReplaceAll(spec.ID, "-", "")
	}
	return spec, nil
}

// decodeScenarioDocument parses one YAML document and reads only the schema discriminator before strict decoding.
func decodeScenarioDocument(body []byte) (*yaml.Node, int, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, 0, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, 0, fmt.Errorf("multiple YAML documents are not supported")
		}
		return nil, 0, fmt.Errorf("decode trailing YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, 0, fmt.Errorf("scenario must be a YAML mapping")
	}

	root := document.Content[0]
	version := 0
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != "schema_version" {
			continue
		}
		if err := root.Content[index+1].Decode(&version); err != nil {
			return nil, 0, fmt.Errorf("schema_version: %w", err)
		}
		break
	}
	return &document, version, nil
}

// decodeKnownScenarioFields retains yaml.v3's strict diagnostics without rewriting scalar content or style.
func decodeKnownScenarioFields(body []byte, destination any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	return decoder.Decode(destination)
}

// rejectScenarioYAMLFeatures keeps v2 deterministic and prevents hidden reuse semantics.
func rejectScenarioYAMLFeatures(document *yaml.Node) error {
	var inspect func(*yaml.Node) error
	inspect = func(node *yaml.Node) error {
		if node.Kind == yaml.AliasNode || node.Anchor != "" {
			return fmt.Errorf("YAML aliases and anchors are not supported")
		}
		if node.Kind == yaml.MappingNode {
			for index := 0; index < len(node.Content); index += 2 {
				if node.Content[index].Value == "<<" {
					return fmt.Errorf("YAML merge keys are not supported")
				}
			}
		}
		for _, child := range node.Content {
			if err := inspect(child); err != nil {
				return err
			}
		}
		return nil
	}
	return inspect(document)
}

// normalizeScenarioV1 preserves the original unversioned representation without inventing live contracts.
func normalizeScenarioV1(wire scenarioSpecV1) ScenarioSpec {
	return ScenarioSpec{
		ID:          wire.ID,
		Title:       wire.Title,
		Description: wire.Description,
		DependsOn:   wire.DependsOn,
		App:         wire.App,
		Markdown:    wire.Markdown,
		Steps:       wire.Steps,
		Verify:      wire.Verify,
	}
}

// normalizeScenarioV2 converts the closed v2 union into the execution model shared with legacy specs.
func normalizeScenarioV2(wire scenarioSpecV2) (ScenarioSpec, error) {
	if wire.SchemaVersion != liveScenarioSchemaVersion {
		return ScenarioSpec{}, fmt.Errorf("schema_version must be %d", liveScenarioSchemaVersion)
	}
	prepareSteps, err := normalizeScenarioV2Steps("prepare.steps", wire.Prepare.Steps)
	if err != nil {
		return ScenarioSpec{}, err
	}
	targetSteps, err := normalizeScenarioV2Steps("steps", wire.Steps)
	if err != nil {
		return ScenarioSpec{}, err
	}
	if err := validateScenarioStepIDs(prepareSteps, targetSteps); err != nil {
		return ScenarioSpec{}, err
	}
	prepareChecks, err := normalizeScenarioV2Checks("prepare.checks", wire.Prepare.Checks)
	if err != nil {
		return ScenarioSpec{}, err
	}
	finalChecks, err := normalizeScenarioV2Checks("checks", wire.Checks)
	if err != nil {
		return ScenarioSpec{}, err
	}
	return ScenarioSpec{
		SchemaVersion: wire.SchemaVersion,
		ID:            wire.ID,
		Title:         wire.Title,
		Description:   wire.Description,
		DependsOn:     wire.DependsOn,
		App:           wire.App,
		Markdown:      wire.Markdown,
		Prepare: ScenarioPreparation{
			Steps:  prepareSteps,
			Checks: prepareChecks,
		},
		Steps:  targetSteps,
		Verify: ScenarioVerification{Commands: finalChecks},
	}, nil
}

// normalizeScenarioV2Steps validates each action before execution can observe any partial plan.
func normalizeScenarioV2Steps(path string, wires []scenarioStepV2) ([]ScenarioStep, error) {
	steps := make([]ScenarioStep, 0, len(wires))
	for index, wire := range wires {
		fieldPath := fmt.Sprintf("%s[%d]", path, index)
		if !scenarioIDPattern.MatchString(wire.ID) {
			return nil, fmt.Errorf("%s.id %q must be a safe slug", fieldPath, wire.ID)
		}
		if strings.TrimSpace(wire.Title) == "" {
			return nil, fmt.Errorf("%s.title is required", fieldPath)
		}
		actions := 0
		if len(wire.Command) > 0 {
			if err := validateScenarioCommand(fieldPath+".command", wire.Command); err != nil {
				return nil, err
			}
			actions++
		}
		if wire.Write != nil {
			if err := validateScenarioFileChange(fieldPath+".write", *wire.Write); err != nil {
				return nil, err
			}
			actions++
		}
		if wire.Append != nil {
			if err := validateScenarioFileChange(fieldPath+".append", *wire.Append); err != nil {
				return nil, err
			}
			actions++
		}
		if wire.Replace != nil {
			if err := validateScenarioReplace(fieldPath+".replace", *wire.Replace); err != nil {
				return nil, err
			}
			actions++
		}
		if actions != 1 {
			return nil, fmt.Errorf("%s must declare exactly one of command, write, append, or replace", fieldPath)
		}
		step := ScenarioStep{ID: wire.ID, Title: wire.Title, Explain: wire.Explain, Write: wire.Write, Append: wire.Append, Replace: wire.Replace}
		if len(wire.Command) > 0 {
			step.Run = &ScenarioCommand{Run: append([]string(nil), wire.Command...)}
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// normalizeScenarioV2Checks validates structured commands before they enter either execution prefix.
func normalizeScenarioV2Checks(path string, wires []scenarioCheckV2) ([]ScenarioCommand, error) {
	checks := make([]ScenarioCommand, 0, len(wires))
	for index, wire := range wires {
		if len(wire.Command) == 0 {
			return nil, fmt.Errorf("%s[%d].command is required", path, index)
		}
		if err := validateScenarioCommand(fmt.Sprintf("%s[%d].command", path, index), wire.Command); err != nil {
			return nil, err
		}
		checks = append(checks, ScenarioCommand{
			Run:      append([]string(nil), wire.Command...),
			Contains: append([]string(nil), wire.Contains...),
		})
	}
	return checks, nil
}

// validateScenarioCommand keeps v2 commands structured by rejecting interpreters that reintroduce a hidden shell language.
func validateScenarioCommand(fieldPath string, arguments []string) error {
	executable := strings.TrimSpace(arguments[0])
	if executable == "" {
		return fmt.Errorf("%s executable is required", fieldPath)
	}
	base := executable
	if separator := strings.LastIndexAny(base, `/\\`); separator >= 0 {
		base = base[separator+1:]
	}
	switch strings.ToLower(base) {
	case "sh", "bash", "zsh", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return fmt.Errorf("%s must not invoke a shell interpreter", fieldPath)
	}
	if strings.ContainsAny(executable, `/\\`) {
		return fmt.Errorf("%s executable must be a tool name, not a path", fieldPath)
	}
	return nil
}

// validateScenarioFileChange rejects unsafe or malformed v2 source before a workspace exists.
func validateScenarioFileChange(fieldPath string, change ScenarioFileChange) error {
	if err := validateScenarioFilePath(fieldPath+".path", change.Path); err != nil {
		return err
	}
	if change.Content == "" {
		return fmt.Errorf("%s.content is required", fieldPath)
	}
	if scenarioGoSource(change.Path, change.Language) {
		if _, err := format.Source([]byte(change.Content)); err != nil {
			return fmt.Errorf("%s.content: invalid Go source: %w", fieldPath, err)
		}
	}
	return nil
}

// validateScenarioReplace ensures a replacement remains an explicit, meaningful transition.
func validateScenarioReplace(fieldPath string, change ScenarioReplace) error {
	if err := validateScenarioFilePath(fieldPath+".path", change.Path); err != nil {
		return err
	}
	if change.Old == "" {
		return fmt.Errorf("%s.old is required", fieldPath)
	}
	if change.New == "" {
		return fmt.Errorf("%s.new is required", fieldPath)
	}
	if change.Old == change.New {
		return fmt.Errorf("%s.new must differ from old", fieldPath)
	}
	return nil
}

// validateScenarioFilePath uses slash semantics so external catalogs are safe on every host OS.
func validateScenarioFilePath(fieldPath, value string) error {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(value), `\\`, "/"))
	volumePath := len(clean) >= 3 && ((clean[0] >= 'a' && clean[0] <= 'z') || (clean[0] >= 'A' && clean[0] <= 'Z')) && clean[1] == ':' && clean[2] == '/'
	if clean == "." || clean == "" || strings.ContainsRune(clean, 0) || strings.HasPrefix(clean, "/") || volumePath || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s %q must be a relative path beneath the scenario root", fieldPath, value)
	}
	return nil
}

// scenarioGoSource recognizes Go edits even when a catalog omitted presentation language metadata.
func scenarioGoSource(filePath, language string) bool {
	return strings.EqualFold(strings.TrimSpace(language), "go") || strings.HasSuffix(strings.ToLower(filePath), ".go")
}

// validateScenarioStepIDs keeps preparation and target actions addressable across narrative edits.
func validateScenarioStepIDs(groups ...[]ScenarioStep) error {
	seen := map[string]bool{}
	for _, steps := range groups {
		for _, step := range steps {
			if seen[step.ID] {
				return fmt.Errorf("duplicate scenario step ID %q", step.ID)
			}
			seen[step.ID] = true
		}
	}
	return nil
}
