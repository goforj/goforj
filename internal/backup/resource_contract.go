package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResourceContractVersion is the current generated App resource contract version.
const ResourceContractVersion = 1

// ResourceContract describes the durable resources owned by one generated App.
type ResourceContract struct {
	Version   int                `json:"version"`
	App       string             `json:"app"`
	Resources []ContractResource `json:"resources"`
}

// ContractResource describes a resource without exposing secret values.
type ContractResource struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Driver     string   `json:"driver"`
	ConfigKeys []string `json:"config_keys"`
	IsDefault  bool     `json:"is_default"`
}

// ErrResourceContractUnavailable indicates that no contract-capable App executable exists.
var ErrResourceContractUnavailable = errors.New("resource contract unavailable")

// ReadResourceContract decodes and validates one App resource contract.
func ReadResourceContract(data []byte, expectedApp string) (ResourceContract, error) {
	var contract ResourceContract
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&contract); err != nil {
		return ResourceContract{}, fmt.Errorf("decode resource contract: %w", err)
	}
	if contract.Version != ResourceContractVersion {
		return ResourceContract{}, fmt.Errorf("unsupported resource contract version %d", contract.Version)
	}
	if strings.TrimSpace(contract.App) == "" {
		return ResourceContract{}, fmt.Errorf("resource contract app is required")
	}
	if expectedApp != "" && contract.App != expectedApp {
		return ResourceContract{}, fmt.Errorf("resource contract app mismatch: expected %s, got %s", expectedApp, contract.App)
	}
	seen := map[string]struct{}{}
	for _, resource := range contract.Resources {
		if strings.TrimSpace(resource.ID) == "" || strings.TrimSpace(resource.Kind) == "" || strings.TrimSpace(resource.Name) == "" || strings.TrimSpace(resource.Driver) == "" {
			return ResourceContract{}, fmt.Errorf("resource contract contains an incomplete resource")
		}
		if resource.Kind != "database" && resource.Kind != "storage" {
			return ResourceContract{}, fmt.Errorf("unsupported resource kind %q", resource.Kind)
		}
		if _, ok := seen[resource.ID]; ok {
			return ResourceContract{}, fmt.Errorf("duplicate resource ID %q", resource.ID)
		}
		seen[resource.ID] = struct{}{}
	}
	return contract, nil
}

// LoadResourceContract executes the selected App's resource command.
func LoadResourceContract(ctx context.Context) (ResourceContract, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	appName := strings.TrimSpace(os.Getenv("FORJ_APP"))
	if appName == "" {
		appName = "app"
	}
	commands := resourceContractCommands(appName)
	for _, command := range commands {
		data, err := executeResourceContract(ctx, command)
		if errors.Is(err, ErrResourceContractUnavailable) {
			continue
		}
		if err != nil {
			return ResourceContract{}, err
		}
		return ReadResourceContract(data, appName)
	}
	return ResourceContract{}, ErrResourceContractUnavailable
}

// ConnectionForResource resolves a database connection through the App contract with a compatibility fallback.
func ConnectionForResource(ctx context.Context, name string) (Connection, error) {
	name = normalizeResourceName(name)
	if name == "" {
		name = "default"
	}
	contract, err := LoadResourceContract(ctx)
	if errors.Is(err, ErrResourceContractUnavailable) {
		return ConnectionFromEnv(name), nil
	}
	if err != nil {
		return Connection{}, err
	}
	for _, resource := range contract.Resources {
		if resource.Kind == "database" && resource.Name == name {
			connection := ConnectionFromEnv(name)
			connection.Driver = resource.Driver
			return connection, nil
		}
	}
	return Connection{}, fmt.Errorf("database resource %q is not present in the App resource contract", name)
}

// resourceContractCommands returns binary and source execution candidates in stable order.
func resourceContractCommands(appName string) [][]string {
	commands := [][]string{}
	binary := filepath.Join("bin", appName)
	if _, err := os.Stat(binary); err == nil {
		commands = append(commands, []string{binary, "resources:describe", "--json"})
	}
	if _, err := os.Stat(filepath.Join("cmd", appName)); err == nil {
		commands = append(commands, []string{"go", "run", "./" + filepath.Join("cmd", appName), "resources:describe", "--json"})
	}
	return commands
}

// executeResourceContract runs one candidate and keeps diagnostics off the JSON stream.
func executeResourceContract(ctx context.Context, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Env = os.Environ()
	if err := command.Run(); err != nil {
		if strings.Contains(strings.ToLower(stderr.String()), "unknown command") || strings.Contains(strings.ToLower(stderr.String()), "no such file") {
			return nil, ErrResourceContractUnavailable
		}
		return nil, fmt.Errorf("run resource contract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("resource contract command returned no JSON")
	}
	return stdout.Bytes(), nil
}
