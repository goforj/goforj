package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// runReference inspects committed applications without changing their bytes or permissions.
func runReference(ctx context.Context, renderer string, projects []string, comparison string) (any, error) {
	renderer, err := filepath.Abs(renderer)
	if err != nil {
		return nil, err
	}
	if comparison != "" {
		comparison, err = filepath.Abs(comparison)
		if err != nil {
			return nil, err
		}
	}
	workspace, err := os.MkdirTemp("/tmp", "goforj-framework-upgrade-")
	if err != nil {
		return nil, err
	}
	results := []map[string]any{}
	for index, project := range projects {
		source, err := filepath.Abs(project)
		if err != nil {
			return nil, err
		}
		var revision, stderr bytes.Buffer
		code, err := runCommand(ctx, source, []string{"git", "rev-parse", "HEAD"}, trialEnvironment(), &revision, &stderr)
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("resolve reference revision: exit code %d: %s", code, stderr.String())
		}
		destination := filepath.Join(workspace, fmt.Sprintf("%d-%s", index, filepath.Base(source)))
		if err := os.Mkdir(destination, 0o755); err != nil {
			return nil, err
		}
		if err := copyCommittedApp(ctx, source, destination, strings.TrimSpace(revision.String())); err != nil {
			return nil, err
		}
		before, err := inventoryTree(destination, false)
		if err != nil {
			return nil, err
		}
		command := []string{renderer, "render", "--plan", "--json"}
		if comparison != "" {
			command = append(command, "--from-renderer", comparison)
		}
		var output bytes.Buffer
		stderr.Reset()
		inspection, cancel := context.WithTimeout(ctx, 90*time.Second)
		code, err = runCommand(inspection, destination, command, trialEnvironment(), &output, &stderr)
		cancel()
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("inspection failed for %s; exit code %d: %s", filepath.Base(source), code, stderr.String())
		}
		var plan struct {
			Scope string                               `json:"scope"`
			Files []struct{ Ownership, Action string } `json:"files"`
		}
		if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
			return nil, err
		}
		if plan.Scope != "framework" || plan.Files == nil {
			return nil, fmt.Errorf("reference renderer requires framework scope and a files array")
		}
		after, err := inventoryTree(destination, false)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(before, after) {
			return nil, fmt.Errorf("inspection changed reference app %s", filepath.Base(source))
		}
		var retained any
		if err := json.Unmarshal(output.Bytes(), &retained); err != nil {
			return nil, err
		}
		if err := writeJSON(filepath.Join(workspace, fmt.Sprintf("%d-%s-plan.json", index, filepath.Base(source))), retained); err != nil {
			return nil, err
		}
		ownerships, actions := map[string]int{}, map[string]int{}
		for _, file := range plan.Files {
			ownerships[file.Ownership]++
			actions[file.Action]++
		}
		results = append(results, map[string]any{"project": filepath.Base(source), "revision": strings.TrimSpace(revision.String()), "scope": plan.Scope, "planned_artifacts": len(plan.Files), "unchanged_bytes_and_modes": true, "ownerships": ownerships, "actions": actions})
	}
	summary := map[string]any{"workspace": workspace, "references": results}
	if err := writeJSON(filepath.Join(workspace, "summary.json"), summary); err != nil {
		return nil, err
	}
	return summary, nil
}
