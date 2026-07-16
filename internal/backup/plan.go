package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Plan describes the native strategy selected for each configured connection.
type Plan struct {
	Resources []PlanResource
	Storage   []StoragePlanResource
}

// PlanResource describes one planned database backup.
type PlanResource struct {
	Connection Connection
	Strategy   string
	Status     string
}

// StoragePlanResource describes one configured storage disk.
type StoragePlanResource struct {
	Name   string
	Driver string
	Root   string
	Prefix string
	Status string
}

// BuildPlan discovers the App-owned database and storage resources.
func BuildPlan() (Plan, error) {
	contract, err := LoadResourceContract(context.Background())
	if err == nil {
		return buildPlanFromContract(contract)
	}
	if err != ErrResourceContractUnavailable {
		return Plan{}, err
	}
	return buildPlanFromEnvironment()
}

// buildPlanFromContract creates a plan from the App-owned resource inventory.
func buildPlanFromContract(contract ResourceContract) (Plan, error) {
	plan := Plan{}
	for _, resource := range contract.Resources {
		switch resource.Kind {
		case "database":
			strategy, err := NativeStrategy(resource.Driver)
			if err != nil {
				return Plan{}, fmt.Errorf("resource %s: %w", resource.ID, err)
			}
			connection := ConnectionFromEnv(resource.Name)
			connection.Driver = resource.Driver
			plan.Resources = append(plan.Resources, PlanResource{Connection: connection, Strategy: strategy.Name(), Status: "backupable"})
		case "storage":
			plan.Storage = append(plan.Storage, StoragePlanResource{
				Name: resource.Name, Driver: resource.Driver,
				Root: storageRootValue(resource.Name), Prefix: storageEnvValue(resource.Name, "PREFIX"),
				Status: storageStatus(resource.Driver),
			})
		}
	}
	if len(plan.Resources) == 0 && len(plan.Storage) == 0 {
		return Plan{}, fmt.Errorf("resource contract contains no backup resources")
	}
	return plan, nil
}

// buildPlanFromEnvironment preserves compatibility with Apps that predate the resource contract.
func buildPlanFromEnvironment() (Plan, error) {
	names := []string{"default"}
	for _, key := range []string{"DB_CONNECTIONS", "DB_SUPPORTED_CONNECTIONS"} {
		for _, name := range strings.Split(os.Getenv(key), ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			if name != "" && name != "default" {
				names = append(names, name)
			}
		}
	}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		if !strings.HasPrefix(key, "DB_") || !strings.HasSuffix(key, "_DRIVER") || key == "DB_DRIVER" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "DB_"), "_DRIVER")
		if name != "" {
			names = append(names, strings.ToLower(name))
		}
	}
	sort.Strings(names[1:])
	plan := Plan{Resources: make([]PlanResource, 0, len(names))}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		connection := ConnectionFromEnv(name)
		strategy, err := NativeStrategy(connection.Driver)
		if err != nil {
			return Plan{}, err
		}
		plan.Resources = append(plan.Resources, PlanResource{
			Connection: connection,
			Strategy:   strategy.Name(),
			Status:     "backupable",
		})
	}
	plan.Storage = discoverStorageResources()
	return plan, nil
}

// storageEnvValue returns a named storage configuration value using the generated environment contract.
func storageEnvValue(name string, field string) string {
	prefix := "STORAGE"
	if name != "default" {
		prefix += "_" + strings.ToUpper(strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(name))
	}
	value := os.Getenv(prefix + "_" + field)
	if value == "" && name != "default" {
		value = os.Getenv("STORAGE_" + field)
	}
	return value
}

// storageRootValue resolves an App storage root while preserving generated App defaults.
func storageRootValue(name string) string {
	if value := storageEnvValue(name, "ROOT"); value != "" {
		return value
	}
	if name == "default" {
		return filepath.Join("storage", "app", "private")
	}
	return filepath.Join("storage", "app", name)
}

// storageStatus classifies storage drivers without claiming unsupported external data is restorable.
func storageStatus(driver string) string {
	if strings.ToLower(strings.TrimSpace(driver)) == "local" || strings.ToLower(strings.TrimSpace(driver)) == "s3" {
		return "backupable"
	}
	return "external-managed"
}

// discoverStorageResources resolves local disks from the generated storage environment contract.
func discoverStorageResources() []StoragePlanResource {
	names := []string{"default"}
	defaultConfigured := os.Getenv("STORAGE_DRIVER") != "" || os.Getenv("STORAGE_ROOT") != ""
	if !defaultConfigured {
		if _, err := os.Stat(filepath.Join("storage", "app", "private")); err != nil {
			names = nil
		}
	}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if !strings.HasPrefix(key, "STORAGE_") || !strings.HasSuffix(key, "_DRIVER") || key == "STORAGE_DRIVER" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "STORAGE_"), "_DRIVER")
		if name != "" {
			names = append(names, strings.ToLower(name))
		}
	}
	if len(names) > 1 {
		sort.Strings(names[1:])
	}
	seen := map[string]bool{}
	resources := []StoragePlanResource{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		prefix := "STORAGE"
		if name != "default" {
			prefix += "_" + strings.ToUpper(name)
		}
		driver := os.Getenv(prefix + "_DRIVER")
		if driver == "" {
			driver = os.Getenv("STORAGE_DRIVER")
		}
		if driver == "" {
			driver = "local"
		}
		root := os.Getenv(prefix + "_ROOT")
		if root == "" && name == "default" {
			root = "storage/app/private"
		}
		if root == "" {
			root = filepath.Join("storage", "app", name)
		}
		status := "external-managed"
		if strings.EqualFold(driver, "local") {
			status = "backupable"
		}
		resources = append(resources, StoragePlanResource{Name: name, Driver: strings.ToLower(driver), Root: root, Prefix: os.Getenv(prefix + "_PREFIX"), Status: status})
	}
	return resources
}

// ConnectionFromEnv resolves one connection using the generated database naming convention.
func ConnectionFromEnv(name string) Connection {
	prefix := "DB"
	if name != "" && name != "default" {
		prefix += "_" + strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(name))
	}
	value := func(suffix string) string {
		if value := os.Getenv(prefix + "_" + suffix); value != "" {
			return value
		}
		return os.Getenv("DB_" + suffix)
	}
	driver := strings.ToLower(strings.TrimSpace(value("DRIVER")))
	if driver == "" {
		driver = "sqlite"
	}
	database := value("DATABASE")
	if database == "" && (driver == "sqlite" || driver == "sqlite3") {
		database = value("SQLITE_DATABASE")
	}
	return Connection{
		Name: name, Driver: driver, DSN: value("DSN"), Database: database,
		Host: value("HOST"), Port: value("PORT"), Username: value("USERNAME"), Password: value("PASSWORD"),
	}
}

// Validate rejects incomplete database or storage entries before artifact creation begins.
func (p Plan) Validate() error {
	if len(p.Resources) == 0 && len(p.Storage) == 0 {
		return fmt.Errorf("backup plan contains no resources")
	}
	for _, resource := range p.Resources {
		if resource.Connection.Name == "" || resource.Strategy == "" {
			return fmt.Errorf("backup plan contains an incomplete database resource")
		}
	}
	for _, resource := range p.Storage {
		if resource.Name == "" || resource.Driver == "" {
			return fmt.Errorf("backup plan contains an incomplete storage resource")
		}
	}
	return nil
}
