package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/launcher"
	"github.com/goforj/goforj/project"
)

const (
	// devStatusSchemaVersion identifies the first machine-readable development status contract.
	devStatusSchemaVersion = 1
	// devStatusMaximumOutputBytes prevents an unexpected runtime from exhausting the caller's memory.
	devStatusMaximumOutputBytes = 1 << 20
	// devStatusMaximumContainers bounds the amount of transient runtime detail admitted into one observation.
	devStatusMaximumContainers = 512
	// devStatusMaximumProblemRunes keeps diagnostic reports safe to persist and display.
	devStatusMaximumProblemRunes = 320
	// devStatusComposeTimeout keeps a blocked container runtime from blocking Harbor reconciliation indefinitely.
	devStatusComposeTimeout = 15 * time.Second
	// devStatusComposeKind identifies services observed through the Compose runtime boundary.
	devStatusComposeKind = "compose"
	// devStatusUnsupportedComposeProblem explains why an owner-customized shell lifecycle cannot be inspected safely.
	devStatusUnsupportedComposeProblem = "custom Compose startup task is not supported"
)

var (
	// errDevStatusOutputTooLarge marks Compose output that exceeds the machine-contract boundary.
	errDevStatusOutputTooLarge = errors.New("Compose status output exceeds the supported size")
)

// DevStatusReport is the stable machine-readable result returned by dev:status.
type DevStatusReport struct {
	SchemaVersion   int                 `json:"schema_version"`
	Supported       bool                `json:"supported"`
	Problem         string              `json:"problem,omitempty"`
	ResourceProblem string              `json:"resource_problem,omitempty"`
	Project         string              `json:"project,omitempty"`
	Services        []DevStatusService  `json:"services"`
	Resources       []DevStatusResource `json:"resources"`
}

// DevStatusService describes the current aggregate state of one Compose service.
type DevStatusService struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Kind       string               `json:"kind"`
	State      string               `json:"state"`
	Active     bool                 `json:"active"`
	Required   bool                 `json:"required"`
	Containers []DevStatusContainer `json:"containers"`
}

// DevStatusContainer describes the bounded runtime facts needed to distinguish service replicas.
type DevStatusContainer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Health   string `json:"health,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// devStatusComposeRunner executes one trusted Compose query without a shell.
type devStatusComposeRunner func(context.Context, string, []string, io.Writer) error

// devStatusComposeCommand holds an executable and arguments selected from an exact conventional task.
type devStatusComposeCommand struct {
	executable string
	arguments  []string
}

// devStatusComposeRecord is the allowlisted subset of Compose JSON admitted into the public report.
type devStatusComposeRecord struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Project  string `json:"Project"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

// devStatusBoundedBuffer caps untrusted subprocess output before JSON decoding begins.
type devStatusBoundedBuffer struct {
	buffer   bytes.Buffer
	maximum  int
	exceeded bool
}

// DevStatusCmd reports current development container state for machine consumers.
type DevStatusCmd struct {
	JSON          bool `name:"json" help:"Print the versioned JSON status contract"`
	ResourcesOnly bool `name:"resources-only" help:"Report host-resolved project resources without querying the container runtime"`

	stdout              io.Writer
	loadProject         devStatusProjectLoader
	resolveResources    devStatusResourceResolver
	run                 devStatusComposeRunner
	launcherEnvironment *launcher.Environment
}

// Signature declares the hidden machine-oriented development status command.
func (*DevStatusCmd) Signature() string {
	return `name:"dev:status" help:"Report development container status as JSON" hidden:""`
}

// Run emits exactly one versioned JSON report for a machine consumer.
func (c *DevStatusCmd) Run() error {
	if !c.JSON {
		return fmt.Errorf("dev:status requires --json")
	}

	report := newDevStatusReport()
	loadProject := c.loadProject
	if loadProject == nil {
		loadProject = func() (devStatusProjectContext, error) {
			return loadDevStatusProject(c.inheritedEnvironment())
		}
	}
	projectContext, err := loadProject()
	if err != nil {
		report.Problem = devStatusProblem("load project development status", err)
		return c.writeReport(report)
	}

	resolveResources := c.resolveResources
	if resolveResources == nil {
		resolveResources = resolveDevStatusResources
	}
	resolvedResources, resolveErr := resolveResources(context.Background(), projectContext.config, projectContext.environment)
	projectResources, validationErr := projectDevStatusResources(resolvedResources)
	report.Resources = projectResources
	if resourceErr := errors.Join(resolveErr, validationErr); resourceErr != nil {
		report.ResourceProblem = devStatusProblem("resolve project resources", resourceErr)
	}
	if c.ResourcesOnly {
		return c.writeReport(report)
	}

	composeCommand, found, supported := selectDevStatusComposeCommand(projectContext.tasks)
	if !found {
		return c.writeReport(report)
	}
	if !supported {
		report.Supported = false
		report.Problem = devStatusUnsupportedComposeProblem
		return c.writeReport(report)
	}

	output := &devStatusBoundedBuffer{maximum: devStatusMaximumOutputBytes}
	run := c.run
	if run == nil {
		run = runDevStatusComposeCommand
	}
	ctx, cancel := context.WithTimeout(context.Background(), devStatusComposeTimeout)
	defer cancel()
	runErr := run(ctx, composeCommand.executable, composeCommand.arguments, output)
	if output.exceeded {
		report.Problem = devStatusProblem("inspect Compose services", errDevStatusOutputTooLarge)
		return c.writeReport(report)
	}
	if runErr != nil {
		report.Problem = devStatusProblem("inspect Compose services", runErr)
		return c.writeReport(report)
	}

	records, err := parseDevStatusComposeOutput(output.Bytes())
	if err != nil {
		report.Problem = devStatusProblem("decode Compose services", err)
		return c.writeReport(report)
	}
	projectName, services, err := aggregateDevStatusServices(records)
	if err != nil {
		report.Problem = devStatusProblem("validate Compose services", err)
		return c.writeReport(report)
	}
	report.Project = projectName
	report.Services = services
	return c.writeReport(report)
}

// inheritedEnvironment returns a private copy of the environment captured at the CLI launcher boundary.
func (c *DevStatusCmd) inheritedEnvironment() processEnvironment {
	launcherEnvironment := c.launcherEnvironment
	if launcherEnvironment == nil {
		launcherEnvironment = launcher.Provide()
	}
	return processEnvironment(launcherEnvironment.Snapshot())
}

// writeReport writes one JSON object while preserving every collection's stable array shape.
func (c *DevStatusCmd) writeReport(report DevStatusReport) error {
	if report.Services == nil {
		report.Services = make([]DevStatusService, 0)
	}
	if report.Resources == nil {
		report.Resources = make([]DevStatusResource, 0)
	}
	stdout := c.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return fmt.Errorf("write dev status report: %w", err)
	}
	return nil
}

// Write rejects output beyond the admitted JSON boundary instead of retaining a partial document.
func (b *devStatusBoundedBuffer) Write(payload []byte) (int, error) {
	if len(payload) > b.maximum-b.buffer.Len() {
		b.exceeded = true
		return 0, errDevStatusOutputTooLarge
	}
	return b.buffer.Write(payload)
}

// Bytes returns the admitted subprocess output.
func (b *devStatusBoundedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

// newDevStatusReport creates an empty supported report with a stable schema shape.
func newDevStatusReport() DevStatusReport {
	return DevStatusReport{
		SchemaVersion: devStatusSchemaVersion,
		Supported:     true,
		Services:      make([]DevStatusService, 0),
		Resources:     make([]DevStatusResource, 0),
	}
}

// selectDevStatusComposeCommand accepts only commands whose tokens are fixed by the GoForj convention.
func selectDevStatusComposeCommand(tasks []project.DevTask) (devStatusComposeCommand, bool, bool) {
	var composeTasks []project.DevTask
	for _, task := range tasks {
		if isDevStatusComposeStartupTask(task) {
			composeTasks = append(composeTasks, task)
		}
	}
	if len(composeTasks) == 0 {
		return devStatusComposeCommand{}, false, true
	}
	if len(composeTasks) != 1 {
		return devStatusComposeCommand{}, true, false
	}
	if strings.TrimSpace(composeTasks[0].Name) != "Run Docker Compose" {
		return devStatusComposeCommand{}, true, false
	}

	switch strings.TrimSpace(composeTasks[0].Cmd) {
	case "docker-compose up -d":
		return devStatusComposeCommand{
			executable: "docker-compose",
			arguments:  []string{"ps", "--all", "--format", "json"},
		}, true, true
	case "docker compose up -d":
		return devStatusComposeCommand{
			executable: "docker",
			arguments:  []string{"compose", "ps", "--all", "--format", "json"},
		}, true, true
	default:
		return devStatusComposeCommand{}, true, false
	}
}

// isDevStatusComposeStartupTask detects Compose startup ownership without interpreting or executing shell syntax.
func isDevStatusComposeStartupTask(task project.DevTask) bool {
	if strings.TrimSpace(task.Name) == "Run Docker Compose" {
		return true
	}
	fields := strings.Fields(strings.TrimSpace(task.Cmd))
	composeArguments := fields
	switch {
	case len(fields) > 0 && fields[0] == "docker-compose":
		composeArguments = fields[1:]
	case len(fields) > 1 && fields[0] == "docker" && fields[1] == "compose":
		composeArguments = fields[2:]
	default:
		return false
	}
	for _, argument := range composeArguments {
		if argument == "up" {
			return true
		}
	}
	return false
}

// runDevStatusComposeCommand invokes the selected Compose CLI directly so task text can never become shell input.
func runDevStatusComposeCommand(ctx context.Context, executable string, arguments []string, stdout io.Writer) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = os.Environ()
	command.Stdout = stdout
	command.Stderr = io.Discard
	return command.Run()
}

// parseDevStatusComposeOutput accepts the array and newline-delimited object formats emitted by supported Compose versions.
func parseDevStatusComposeOutput(source []byte) ([]devStatusComposeRecord, error) {
	trimmed := bytes.TrimSpace(source)
	if len(trimmed) == 0 {
		return make([]devStatusComposeRecord, 0), nil
	}
	if trimmed[0] == '[' {
		var records []devStatusComposeRecord
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		if err := decoder.Decode(&records); err != nil {
			return nil, fmt.Errorf("decode JSON array: %w", err)
		}
		if records == nil {
			return nil, fmt.Errorf("Compose status array is null")
		}
		if err := requireDevStatusJSONEnd(decoder); err != nil {
			return nil, err
		}
		return records, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	records := make([]devStatusComposeRecord, 0)
	for {
		var record devStatusComposeRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode newline-delimited JSON: %w", err)
		}
		records = append(records, record)
		if len(records) > devStatusMaximumContainers {
			return nil, fmt.Errorf("Compose status contains more than %d containers", devStatusMaximumContainers)
		}
	}
	return records, nil
}

// requireDevStatusJSONEnd rejects additional JSON values after an array document.
func requireDevStatusJSONEnd(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return fmt.Errorf("Compose status contains multiple JSON documents")
}

// aggregateDevStatusServices validates allowlisted fields and combines replicas by Compose service identity.
func aggregateDevStatusServices(records []devStatusComposeRecord) (string, []DevStatusService, error) {
	if len(records) > devStatusMaximumContainers {
		return "", nil, fmt.Errorf("Compose status contains more than %d containers", devStatusMaximumContainers)
	}
	if len(records) == 0 {
		return "", make([]DevStatusService, 0), nil
	}

	projectName := ""
	seenContainers := make(map[string]struct{}, len(records))
	servicesByID := make(map[string]*DevStatusService)
	for index, record := range records {
		normalized, err := normalizeDevStatusComposeRecord(record)
		if err != nil {
			return "", nil, fmt.Errorf("container %d: %w", index+1, err)
		}
		if projectName == "" {
			projectName = normalized.Project
		} else if normalized.Project != projectName {
			return "", nil, fmt.Errorf("containers belong to multiple Compose projects")
		}
		if _, exists := seenContainers[normalized.ID]; exists {
			return "", nil, fmt.Errorf("container IDs are not unique")
		}
		seenContainers[normalized.ID] = struct{}{}

		service, exists := servicesByID[normalized.Service]
		if !exists {
			service = &DevStatusService{
				ID:         normalized.Service,
				Name:       normalized.Service,
				Kind:       devStatusComposeKind,
				Containers: make([]DevStatusContainer, 0),
			}
			servicesByID[normalized.Service] = service
		}
		service.Containers = append(service.Containers, DevStatusContainer{
			ID:       normalized.ID,
			Name:     normalized.Name,
			State:    normalized.State,
			Health:   normalized.Health,
			ExitCode: normalized.ExitCode,
		})
	}

	serviceIDs := make([]string, 0, len(servicesByID))
	for serviceID := range servicesByID {
		serviceIDs = append(serviceIDs, serviceID)
	}
	sort.Strings(serviceIDs)
	services := make([]DevStatusService, 0, len(serviceIDs))
	for _, serviceID := range serviceIDs {
		service := servicesByID[serviceID]
		sort.Slice(service.Containers, func(left int, right int) bool {
			if service.Containers[left].Name == service.Containers[right].Name {
				return service.Containers[left].ID < service.Containers[right].ID
			}
			return service.Containers[left].Name < service.Containers[right].Name
		})
		service.State, service.Active = aggregateDevStatusServiceState(service.Containers)
		services = append(services, *service)
	}
	return projectName, services, nil
}

// normalizeDevStatusComposeRecord admits only identifiers and states needed by the public status contract.
func normalizeDevStatusComposeRecord(record devStatusComposeRecord) (devStatusComposeRecord, error) {
	if !validDevStatusIdentifier(record.ID, 128) {
		return devStatusComposeRecord{}, fmt.Errorf("ID is missing or unsafe")
	}
	if !validDevStatusIdentifier(record.Name, 256) {
		return devStatusComposeRecord{}, fmt.Errorf("name is missing or unsafe")
	}
	if !validDevStatusIdentifier(record.Project, 128) {
		return devStatusComposeRecord{}, fmt.Errorf("project is missing or unsafe")
	}
	if !validDevStatusIdentifier(record.Service, 128) {
		return devStatusComposeRecord{}, fmt.Errorf("service is missing or unsafe")
	}

	record.State = strings.ToLower(strings.TrimSpace(record.State))
	switch record.State {
	case "created", "dead", "exited", "paused", "removing", "restarting", "running", "unknown":
	default:
		return devStatusComposeRecord{}, fmt.Errorf("state is missing or unsupported")
	}
	record.Health = strings.ToLower(strings.TrimSpace(record.Health))
	switch record.Health {
	case "", "healthy", "starting", "unhealthy", "unknown":
	default:
		return devStatusComposeRecord{}, fmt.Errorf("health is unsupported")
	}
	if record.ExitCode < 0 || record.ExitCode > 255 {
		return devStatusComposeRecord{}, fmt.Errorf("exit code is outside the supported range")
	}
	return record, nil
}

// validDevStatusIdentifier constrains runtime identifiers to Compose's portable non-shell token vocabulary.
func validDevStatusIdentifier(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

// aggregateDevStatusServiceState maps replica states onto Harbor's stable service-state vocabulary.
func aggregateDevStatusServiceState(containers []DevStatusContainer) (string, bool) {
	ready := 0
	working := 0
	degraded := 0
	failed := 0
	stopped := 0
	active := false
	for _, container := range containers {
		state, containerActive := devStatusContainerServiceState(container)
		active = active || containerActive
		switch state {
		case "ready":
			ready++
		case "working":
			working++
		case "degraded":
			degraded++
		case "failed":
			failed++
		case "stopped":
			stopped++
		}
	}

	switch {
	case failed > 0:
		return "failed", active
	case degraded > 0 || ready > 0 && stopped > 0:
		return "degraded", active
	case working > 0:
		return "working", active
	case ready > 0:
		return "ready", active
	case stopped > 0:
		return "stopped", active
	default:
		return "unavailable", active
	}
}

// devStatusContainerServiceState converts one Compose record into the aggregate service vocabulary.
func devStatusContainerServiceState(container DevStatusContainer) (string, bool) {
	switch container.State {
	case "running":
		switch container.Health {
		case "unhealthy":
			return "degraded", true
		case "starting":
			return "working", true
		case "unknown":
			return "degraded", true
		default:
			return "ready", true
		}
	case "restarting":
		return "working", true
	case "paused":
		return "degraded", true
	case "created", "removing":
		return "working", false
	case "exited":
		if container.ExitCode == 0 {
			return "stopped", false
		}
		return "failed", false
	case "dead":
		return "failed", false
	default:
		return "unavailable", false
	}
}

// devStatusProblem creates one bounded single-line diagnostic without copying subprocess output into the contract.
func devStatusProblem(operation string, err error) string {
	problem := strings.ToValidUTF8(operation+": "+err.Error(), "?")
	problem = strings.Join(strings.Fields(problem), " ")
	runes := []rune(problem)
	if len(runes) > devStatusMaximumProblemRunes {
		problem = string(runes[:devStatusMaximumProblemRunes-1]) + "…"
	}
	return problem
}
