package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/appassets"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

const (
	conventionalSPAInstallCommand = "npm install --no-audit --no-fund --loglevel=error"
	conventionalSPABuildCommand   = "npm run build -s -- --logLevel error"
)

// configuredAppAssets resolves every explicit SPA ownership record without adding a second build configuration surface.
func configuredAppAssets(root string) ([]appassets.Asset, error) {
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load project config for frontend assets: %w", err)
	}
	appNames := make([]string, 0, len(config.Dev.Apps))
	for appName := range config.Dev.Apps {
		appNames = append(appNames, appName)
	}
	sort.Strings(appNames)
	assets := make([]appassets.Asset, 0)
	for _, appName := range appNames {
		configured := config.Dev.Apps[appName]
		names := make([]string, 0, len(configured.SPAs))
		for name := range configured.SPAs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			spa := configured.SPAs[name]
			command := strings.TrimSpace(spa.Build)
			if command == "" {
				command = conventionalSPABuildCommand
			}
			prepare := ""
			if usesNPMCommand(command) {
				prepare = conventionalSPAInstallCommand
			}
			assets = append(assets, appassets.Asset{
				App:     appName,
				Name:    name,
				Root:    spa.Path,
				Prepare: prepare,
				Command: command,
			})
		}
	}
	return assets, nil
}

// usesNPMCommand limits automatic dependency setup to SPAs whose configured build explicitly selects npm.
func usesNPMCommand(command string) bool {
	fields := strings.Fields(command)
	return len(fields) > 0 && fields[0] == "npm"
}

// prepareAppAssets builds stale SPAs and records their metadata only after successful output publication.
func prepareAppAssets(root string, assets []appassets.Asset) (string, error) {
	return prepareAppAssetsWithRunner(root, assets, runAppAssetCommand)
}

// assetCommandRunner isolates phased subprocess execution from freshness and publication tests.
type assetCommandRunner func(root string, asset appassets.Asset, phase string, command string) error

// prepareAppAssetsWithRunner applies the asset transaction around an injectable command boundary.
func prepareAppAssetsWithRunner(root string, assets []appassets.Asset, run assetCommandRunner) (string, error) {
	if len(assets) == 0 {
		return "", nil
	}
	built := 0
	for _, asset := range assets {
		current, err := appassets.Current(root, asset)
		if err != nil {
			return "", fmt.Errorf("inspect SPA %s/%s: %w", asset.App, asset.Name, err)
		}
		if current {
			continue
		}
		if strings.TrimSpace(asset.Prepare) != "" {
			if err := run(root, asset, "prepare", asset.Prepare); err != nil {
				return "", fmt.Errorf("prepare SPA %s/%s dependencies: %w", asset.App, asset.Name, err)
			}
		}
		if err := run(root, asset, "build", asset.Command); err != nil {
			return "", fmt.Errorf("build SPA %s/%s: %w", asset.App, asset.Name, err)
		}
		if err := appassets.Record(root, asset); err != nil {
			return "", fmt.Errorf("record SPA %s/%s build: %w", asset.App, asset.Name, err)
		}
		built++
	}
	if built == 0 {
		return "current", nil
	}
	if built == 1 {
		return "1 built", nil
	}
	return fmt.Sprintf("%d built", built), nil
}

// runAppAssetCommand executes one frontend phase through the shared cross-platform process supervisor.
func runAppAssetCommand(root string, asset appassets.Asset, phase string, command string) error {
	supervisor := devwatch.NewSupervisor(devwatch.SupervisorOptions{})
	defer supervisor.Close()
	stdout, stderr, reveal := appAssetCommandWriters(phase, os.Stdout, os.Stderr)
	exit, err := supervisor.Run(context.Background(), phase+" SPA "+asset.App+"/"+asset.Name, devwatch.Command{
		Shell:  command,
		Dir:    rootedBuildPath(root, asset.Root),
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		reveal()
		return err
	}
	if !exit.OK() {
		reveal()
		return fmt.Errorf("exited with code %d", exit.ExitCode)
	}
	return nil
}

// appAssetCommandWriters buffers dependency setup while leaving build output visible and returns a failure reveal function.
func appAssetCommandWriters(phase string, visibleStdout io.Writer, visibleStderr io.Writer) (io.Writer, io.Writer, func()) {
	if phase != "prepare" {
		return visibleStdout, visibleStderr, func() {}
	}
	var capturedStdout bytes.Buffer
	var capturedStderr bytes.Buffer
	reveal := func() {
		_, _ = capturedStdout.WriteTo(visibleStdout)
		_, _ = capturedStderr.WriteTo(visibleStderr)
	}
	return &capturedStdout, &capturedStderr, reveal
}
