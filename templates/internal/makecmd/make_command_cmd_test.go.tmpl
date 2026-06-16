package makecmd

import (
	"path/filepath"
	"testing"
)

func TestCommandTargetUsesLowercaseSignatureName(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "bare pascal command", raw: "Wow", want: "wow"},
		{name: "grouped pascal command", raw: "BillingPortal:UsageReports:SyncNow", want: "billing-portal:usage-reports:sync-now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &CommandCmd{}
			target := cmd.resolveCommandTargetFromName(tt.raw)
			if target.commandName != tt.want {
				t.Fatalf("command name = %q, want %q", target.commandName, tt.want)
			}
		})
	}
}

func TestCommandTargetNormalizesExplicitSignatureName(t *testing.T) {
	cmd := &CommandCmd{CmdName: "Reports:SyncNow"}
	target := cmd.resolveCommandTargetFromName("Sync")
	if target.commandName != "reports:sync-now" {
		t.Fatalf("command name = %q, want reports:sync-now", target.commandName)
	}
}

func TestCommandCmdRemoveDeletesFileAndWiring(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	writeMakeCmdTestFile(t, "go.mod", "module example.com/testapp\n\ngo 1.24\n")
	writeMakeCmdTestFile(t, filepath.Join("internal", "cmd", "wire.go"), `package cmd

import "github.com/goforj/wire"

var AppCommandSet = wire.NewSet(
)
`)
	writeMakeCmdTestFile(t, filepath.Join("internal", "cmd", "app_commands.go"), `package cmd

type Commands struct {
}

func NewCommands(
) *Commands {
	return &Commands{
	}
}
`)

	create := &CommandCmd{Name: "Reports:Sync", OutputDir: defaultCommandOutputDir}
	if err := create.Run(); err != nil {
		t.Fatalf("create command: %v", err)
	}

	outputPath := filepath.Join("internal", "reports", "sync_cmd.go")
	assertMakeCmdTestContains(t, outputPath, []string{
		"package reports",
		`name:"reports:sync"`,
	})
	assertMakeCmdTestContains(t, filepath.Join("internal", "cmd", "wire.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewSyncCmd",
	})
	assertMakeCmdTestContains(t, filepath.Join("internal", "cmd", "app_commands.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"ReportsSyncCmd reports.SyncCmd",
		"reportsSyncCmd *reports.SyncCmd",
	})

	remove := &CommandCmd{Name: "Reports:Sync", OutputDir: defaultCommandOutputDir, Remove: true}
	if err := remove.Run(); err != nil {
		t.Fatalf("remove command: %v", err)
	}

	assertMakeCmdTestFileMissing(t, outputPath)
	assertMakeCmdTestNotContains(t, filepath.Join("internal", "cmd", "wire.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewSyncCmd",
	})
	assertMakeCmdTestNotContains(t, filepath.Join("internal", "cmd", "app_commands.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"ReportsSyncCmd reports.SyncCmd",
		"reportsSyncCmd *reports.SyncCmd",
	})
}

func TestCommandCmdUsesActiveApp(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("FORJ_APP", "reporting")

	writeMakeCmdTestFile(t, "go.mod", "module example.com/testapp\n\ngo 1.24\n")
	writeMakeCmdTestFile(t, filepath.Join("app", "wire", "inject_cmd_app.go"), `package wire

import "github.com/goforj/wire"

var appCommandSet = wire.NewSet(
)
`)
	writeMakeCmdTestFile(t, filepath.Join("app", "commands.go"), `package app

type Commands struct {
}

func NewCommands(
) *Commands {
	return &Commands{
	}
}
`)
	writeMakeCmdTestFile(t, filepath.Join("app", "reporting", "wire", "inject_cmd_app.go"), `package wire

import "github.com/goforj/wire"

var appCommandSet = wire.NewSet(
)
`)
	writeMakeCmdTestFile(t, filepath.Join("app", "reporting", "commands.go"), `package reportingapp

type Commands struct {
}

func NewCommands(
) *Commands {
	return &Commands{
	}
}
`)

	cmd := &CommandCmd{Name: "Reports:Sync", OutputDir: defaultCommandOutputDir}
	if err := cmd.Run(); err != nil {
		t.Fatalf("create targeted command: %v", err)
	}

	assertMakeCmdTestContains(t, filepath.Join("app", "reporting", "wire", "inject_cmd_app.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewSyncCmd",
	})
	assertMakeCmdTestContains(t, filepath.Join("app", "reporting", "commands.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"ReportsSyncCmd reports.SyncCmd",
		"reportsSyncCmd *reports.SyncCmd",
	})
	assertMakeCmdTestNotContains(t, filepath.Join("app", "wire", "inject_cmd_app.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewSyncCmd",
	})
	assertMakeCmdTestNotContains(t, filepath.Join("app", "commands.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"ReportsSyncCmd reports.SyncCmd",
		"reportsSyncCmd *reports.SyncCmd",
	})
}
