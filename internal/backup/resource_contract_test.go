package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadResourceContractValidatesVersionAndResourceShape(t *testing.T) {
	contract, err := ReadResourceContract([]byte(`{"version":1,"app":"billing","resources":[{"id":"db.reporting","kind":"database","name":"reporting","driver":"postgres","config_keys":["DB_REPORTING_PASSWORD"]}]}`), "billing")
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Resources) != 1 || contract.Resources[0].Name != "reporting" {
		t.Fatalf("contract = %#v", contract)
	}
	if strings.Contains(string([]byte(`{"version":1,"app":"billing","resources":[{"id":"db.reporting","kind":"database","name":"reporting","driver":"postgres","config_keys":["DB_REPORTING_PASSWORD"]}]}`)), "secret-value") {
		t.Fatal("contract test fixture unexpectedly contains a secret value")
	}
}

func TestReadResourceContractRejectsInvalidContracts(t *testing.T) {
	for name, data := range map[string]string{
		"malformed":        `{`,
		"wrong version":    `{"version":2,"app":"billing","resources":[]}`,
		"unsupported kind": `{"version":1,"app":"billing","resources":[{"id":"queue.default","kind":"queue","name":"default","driver":"redis"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadResourceContract([]byte(data), "billing"); err == nil {
				t.Fatal("expected contract validation error")
			}
		})
	}
}

func TestLoadResourceContractUsesNamedAppBinary(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	t.Setenv("FORJ_APP", "billing")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' '{"version":1,"app":"billing","resources":[{"id":"storage.public","kind":"storage","name":"public","driver":"local","config_keys":["STORAGE_PUBLIC_ROOT"]}]}'
`
	path := filepath.Join(root, "bin", "billing")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	contract, err := LoadResourceContract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if contract.App != "billing" || contract.Resources[0].ID != "storage.public" {
		t.Fatalf("contract = %#v", contract)
	}
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Storage) != 1 || plan.Storage[0].Name != "public" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanUsesContractDatabaseAndStorageResources(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	t.Setenv("FORJ_APP", "billing")
	t.Setenv("DB_REPORTING_DRIVER", "postgres")
	t.Setenv("STORAGE_ASSETS_DRIVER", "local")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' '{"version":1,"app":"billing","resources":[{"id":"db.default","kind":"database","name":"default","driver":"sqlite"},{"id":"db.reporting","kind":"database","name":"reporting","driver":"postgres"},{"id":"storage.default","kind":"storage","name":"default","driver":"local"},{"id":"storage.assets","kind":"storage","name":"assets","driver":"local"}]}'
`
	if err := os.WriteFile(filepath.Join(root, "bin", "billing"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Resources) != 2 || plan.Resources[1].Connection.Name != "reporting" {
		t.Fatalf("database plan = %#v", plan.Resources)
	}
	if len(plan.Storage) != 2 || plan.Storage[1].Name != "assets" {
		t.Fatalf("storage plan = %#v", plan.Storage)
	}
}
