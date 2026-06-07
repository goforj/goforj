package build

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const profileToolCommand = "__forj_build_profile_exec"

type CompileProfileEntry struct {
	Package     string   `json:"package"`
	DurationMS  int64    `json:"duration_ms"`
	Invocations int      `json:"invocations"`
	ImportChain []string `json:"import_chain,omitempty"`
}

type CompileProfileReport struct {
	BaselineTotalMS int64                 `json:"baseline_total_ms"`
	ProfiledTotalMS int64                 `json:"profiled_total_ms"`
	Entries         []CompileProfileEntry `json:"entries"`
}

type goBuildOptions struct {
	extraEnv      []string
	forceNoCache  bool
	allowRecovery bool
}

func HandleProfileTool(args []string) bool {
	if len(args) == 0 || args[0] != profileToolCommand {
		return false
	}
	os.Exit(runProfileTool(args[1:]))
	return true
}

func runProfileTool(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing tool executable")
		return 2
	}
	toolPath := args[0]
	toolArgs := args[1:]
	cmd := exec.Command(toolPath, toolArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)

	if filepath.Base(toolPath) == "compile" || filepath.Base(toolPath) == "compile.exe" {
		pkg := compilePackageName(toolArgs)
		if pkg != "" {
			_ = appendCompileProfile(os.Getenv("FORJ_BUILD_PROFILE_LOG"), pkg, duration)
		}
	}

	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func (c *Cmd) buildBinaryWithProfile(args []string) (string, error) {
	return c.buildBinaryWithCompileProfile(args)
}

func (c *Cmd) printProfile() error {
	fmt.Fprintln(os.Stdout, "forj build profile")
	return printCompileProfile(os.Stdout, c.compileProfile, c.Top)
}

func (c *Cmd) buildBinaryWithCompileProfile(args []string) (string, error) {
	logFile, err := os.CreateTemp("", "forj-build-compile-profile-*.log")
	if err != nil {
		return "", err
	}
	logPath := logFile.Name()
	_ = logFile.Close()
	defer os.Remove(logPath)

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	baselineStartedAt := time.Now()
	if _, err := c.runGoBuild(args, goBuildOptions{forceNoCache: true}); err != nil {
		return "", err
	}
	baselineTotalMS := time.Since(baselineStartedAt).Milliseconds()

	buildArgs := []string{"build", "-toolexec", exePath + " " + profileToolCommand, "-a"}
	buildArgs = append(buildArgs, args...)
	profiledStartedAt := time.Now()
	if _, err := c.runGoBuild(buildArgs[1:], goBuildOptions{
		extraEnv: []string{"FORJ_BUILD_PROFILE_LOG=" + logPath},
	}); err != nil {
		return "", err
	}
	profiledTotalMS := time.Since(profiledStartedAt).Milliseconds()

	report, err := loadCompileProfile(logPath)
	if err != nil {
		return "", err
	}
	c.compileProfile = normalizeCompileProfile(report, baselineTotalMS, profiledTotalMS)
	root, err := filepath.Abs(c.Root)
	if err == nil {
		annotateCompileProfile(root, &c.compileProfile)
	}
	return "", nil
}

func (c *Cmd) runPlainGoBuild(args []string) (string, error) {
	c.lastBuildStatus = ""
	if atomicArgs, output, cleanup, ok, err := atomicGoBuildArgs(args); ok || err != nil {
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			return "", err
		}
		if _, err := c.runGoBuild(atomicArgs, goBuildOptions{allowRecovery: true}); err != nil {
			return "", err
		}
		if err := os.Chmod(output.temp, 0o755); err != nil {
			return "", fmt.Errorf("prepare built binary permissions: %w", err)
		}
		if err := os.Rename(output.temp, output.final); err != nil {
			return "", fmt.Errorf("publish built binary: %w", err)
		}
		return c.lastBuildStatus, nil
	}
	if _, err := c.runGoBuild(args, goBuildOptions{allowRecovery: true}); err != nil {
		return "", err
	}
	return c.lastBuildStatus, nil
}

type atomicBuildOutput struct {
	final string
	temp  string
}

// atomicGoBuildArgs builds watched binaries away from their final path so dev never executes a partial file.
func atomicGoBuildArgs(args []string) ([]string, atomicBuildOutput, func(), bool, error) {
	outIndex := outputArgIndex(args)
	if outIndex < 0 {
		return args, atomicBuildOutput{}, nil, false, nil
	}
	final := outputPath(args[outIndex])
	if !atomicBuildOutputPath(final) {
		return args, atomicBuildOutput{}, nil, false, nil
	}
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, atomicBuildOutput{}, nil, true, err
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(final)+".tmp-*")
	if err != nil {
		return nil, atomicBuildOutput{}, nil, true, err
	}
	temp := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return nil, atomicBuildOutput{}, nil, true, err
	}
	if err := os.Remove(temp); err != nil {
		return nil, atomicBuildOutput{}, nil, true, err
	}
	atomicArgs := replaceBuildOutputArg(args, outIndex, temp)
	cleanup := func() { _ = os.Remove(temp) }
	return atomicArgs, atomicBuildOutput{final: final, temp: temp}, cleanup, true, nil
}

// atomicBuildOutputPath avoids rewriting cases where go build expects -o to be a directory.
func atomicBuildOutputPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return false
	}
	if strings.HasSuffix(path, string(os.PathSeparator)) || strings.HasSuffix(path, "/") {
		return false
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return false
	}
	return true
}

func replaceBuildOutputArg(args []string, outIndex int, output string) []string {
	out := append([]string(nil), args...)
	if strings.HasPrefix(out[outIndex], "-o=") {
		out[outIndex] = "-o=" + output
		return out
	}
	out[outIndex] = output
	return out
}

func (c *Cmd) runGoBuild(args []string, opts goBuildOptions) (bool, error) {
	buildArgs := append([]string{"build"}, args...)
	if opts.forceNoCache {
		buildArgs = append([]string{"build", "-a"}, args...)
	}
	cmd := exec.Command("go", buildArgs...)
	if len(opts.extraEnv) > 0 {
		cmd.Env = append(os.Environ(), opts.extraEnv...)
	}
	if debugEnabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("go build: %w", err)
		}
		return false, nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if opts.allowRecovery {
			recovered, recoverErr := c.attemptMissingModuleRecovery(detail)
			if recoverErr == nil && recovered {
				opts.allowRecovery = false
				return c.runGoBuild(args, opts)
			}
		}
		if detail != "" {
			return false, fmt.Errorf("go build: %w (%s)", err, detail)
		}
		return false, fmt.Errorf("go build: %w", err)
	}
	return false, nil
}

var missingModulePattern = regexp.MustCompile(`no required module provides package (\S+?)(?:;|\s|$)`)

func missingModulePackages(detail string) []string {
	matches := missingModulePattern.FindAllStringSubmatch(detail, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	pkgs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		pkg := strings.TrimSpace(match[1])
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func (c *Cmd) attemptMissingModuleRecovery(detail string) (bool, error) {
	missingPkgs := missingModulePackages(detail)
	if len(missingPkgs) == 0 {
		return false, nil
	}
	if err := c.runGoGet(missingPkgs); err != nil {
		return false, err
	}
	c.lastBuildStatus = "synced deps, retried"
	return true, nil
}

func (c *Cmd) runGoGet(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	if c.goGetFunc != nil {
		return c.goGetFunc(packages)
	}
	args := append([]string{"get"}, packages...)
	cmd := exec.Command("go", args...)
	if debugEnabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go get: %w", err)
		}
		return nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return fmt.Errorf("go get: %w (%s)", err, detail)
		}
		return fmt.Errorf("go get: %w", err)
	}
	return nil
}

func compilePackageName(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-p" {
			return args[i+1]
		}
	}
	return ""
}

func appendCompileProfile(path, pkg string, duration time.Duration) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%d\n", pkg, duration.Milliseconds())
	return err
}

func loadCompileProfile(path string) (CompileProfileReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return CompileProfileReport{}, err
	}
	defer f.Close()

	totals := map[string]CompileProfileEntry{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		var ms int64
		if _, err := fmt.Sscanf(parts[1], "%d", &ms); err != nil {
			continue
		}
		entry := totals[parts[0]]
		entry.Package = parts[0]
		entry.DurationMS += ms
		entry.Invocations++
		totals[parts[0]] = entry
	}
	if err := scanner.Err(); err != nil {
		return CompileProfileReport{}, err
	}

	report := CompileProfileReport{Entries: make([]CompileProfileEntry, 0, len(totals))}
	for _, entry := range totals {
		report.Entries = append(report.Entries, entry)
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		if report.Entries[i].DurationMS != report.Entries[j].DurationMS {
			return report.Entries[i].DurationMS > report.Entries[j].DurationMS
		}
		return report.Entries[i].Package < report.Entries[j].Package
	})
	return report, nil
}

func printCompileProfile(w io.Writer, report CompileProfileReport, top int) error {
	if len(report.Entries) == 0 {
		_, err := fmt.Fprintln(w, "No packages were compiled in this build.")
		return err
	}
	if report.BaselineTotalMS > 0 {
		if _, err := fmt.Fprintf(w, "Baseline build total: %dms\n", report.BaselineTotalMS); err != nil {
			return err
		}
	}
	if report.ProfiledTotalMS > 0 {
		if _, err := fmt.Fprintf(w, "Profiled build total: %dms\n", report.ProfiledTotalMS); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "Compile time (packages compiled in this build):"); err != nil {
		return err
	}
	limit := len(report.Entries)
	if top > 0 && top < limit {
		limit = top
	}
	for i := 0; i < limit; i++ {
		entry := report.Entries[i]
		if _, err := fmt.Fprintf(w, "  %2d. %-40s %4dms", i+1, entry.Package, entry.DurationMS); err != nil {
			return err
		}
		if entry.Invocations > 1 {
			if _, err := fmt.Fprintf(w, " (%dx)", entry.Invocations); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if len(entry.ImportChain) > 1 {
			printImportChain(w, entry.ImportChain)
		}
	}
	if limit < len(report.Entries) {
		_, err := fmt.Fprintf(w, "      ... %d more packages omitted\n", len(report.Entries)-limit)
		return err
	}
	return nil
}

func normalizeCompileProfile(report CompileProfileReport, baselineTotalMS, profiledTotalMS int64) CompileProfileReport {
	report.BaselineTotalMS = baselineTotalMS
	report.ProfiledTotalMS = profiledTotalMS
	if baselineTotalMS <= 0 || profiledTotalMS <= 0 || len(report.Entries) == 0 {
		return report
	}
	for i := range report.Entries {
		report.Entries[i].DurationMS = report.Entries[i].DurationMS * baselineTotalMS / profiledTotalMS
	}
	return report
}

type goListPackage struct {
	ImportPath string
	Imports    []string
}

type importLoadResult struct {
	packages map[string]goListPackage
	roots    []string
}

func annotateCompileProfile(root string, report *CompileProfileReport) {
	loaded, err := loadImportPackages(root, defaultAnalyzePatterns(root))
	if err != nil {
		return
	}
	for i := range report.Entries {
		report.Entries[i].ImportChain = importChainToTarget(loaded, report.Entries[i].Package)
	}
}

func loadImportPackages(root string, patterns []string) (importLoadResult, error) {
	args := append([]string{"list", "-deps", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOCACHE=/tmp/gocache", "GOMODCACHE=/tmp/gomodcache")

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return importLoadResult{}, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return importLoadResult{}, err
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	result := importLoadResult{
		packages: map[string]goListPackage{},
		roots:    make([]string, 0, len(patterns)),
	}
	for {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return importLoadResult{}, err
		}
		if pkg.ImportPath == "" {
			continue
		}
		result.packages[pkg.ImportPath] = pkg
	}
	for _, pattern := range patterns {
		rootPkgs, err := loadRootPackages(root, pattern)
		if err != nil {
			return importLoadResult{}, err
		}
		result.roots = append(result.roots, rootPkgs...)
	}
	return result, nil
}

func loadRootPackages(root string, pattern string) ([]string, error) {
	cmd := exec.Command("go", "list", pattern)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOCACHE=/tmp/gocache", "GOMODCACHE=/tmp/gomodcache")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	lines := strings.Fields(string(out))
	return lines, nil
}

func importChainToTarget(loaded importLoadResult, target string) []string {
	if len(loaded.roots) == 0 {
		return nil
	}
	if _, ok := loaded.packages[target]; !ok {
		return nil
	}

	queue := append([]string(nil), loaded.roots...)
	parents := map[string]string{}
	seen := map[string]struct{}{}
	for _, root := range loaded.roots {
		seen[root] = struct{}{}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			break
		}
		pkg, ok := loaded.packages[current]
		if !ok {
			continue
		}
		for _, next := range pkg.Imports {
			if _, ok := loaded.packages[next]; !ok {
				continue
			}
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			parents[next] = current
			queue = append(queue, next)
		}
	}

	if _, ok := seen[target]; !ok {
		return nil
	}
	var chain []string
	current := target
	chain = append(chain, current)
	for {
		parent, ok := parents[current]
		if !ok {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func defaultAnalyzePatterns(root string) []string {
	var resolved []string
	if dirExists(filepath.Join(root, "internal")) {
		resolved = append(resolved, "./internal/...")
	}
	if dirExists(filepath.Join(root, "app")) {
		resolved = append(resolved, "./app/...")
	}
	if dirExists(filepath.Join(root, "wire")) {
		resolved = append(resolved, "./wire")
	}
	if len(resolved) == 0 {
		return []string{"."}
	}
	return resolved
}

func printImportChain(w io.Writer, chain []string) {
	for i, part := range chain {
		indent := "      "
		if i > 0 {
			indent += strings.Repeat("   ", i-1)
		}
		fmt.Fprintf(w, "%s└─ %s\n", indent, part)
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
