package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/goforj/goforj/internal/compileprofile"
)

const profileToolCommand = "__forj_build_profile_exec"

// atomicBuildOutputSequence avoids relying on host clock resolution for overlapping build identity.
var atomicBuildOutputSequence atomic.Uint64

type goBuildOptions struct {
	extraEnv      []string
	forceNoCache  bool
	allowRecovery bool
}

// HandleProfileTool intercepts Go's toolexec callback before normal CLI initialization can alter compiler behavior.
func HandleProfileTool(args []string) bool {
	if len(args) == 0 || args[0] != profileToolCommand {
		return false
	}
	os.Exit(runProfileTool(args[1:]))
	return true
}

// runProfileTool delegates to the requested Go tool while recording compile durations without changing its exit semantics.
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
			_ = compileprofile.Record(os.Getenv("FORJ_BUILD_PROFILE_LOG"), pkg, duration)
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

// buildBinaryWithProfile compiles from the selected project while collecting package timing data.
func (c *Cmd) buildBinaryWithProfile(root string, args []string) (string, error) {
	return c.buildBinaryWithCompileProfile(root, args)
}

// printProfile keeps command-owned headings separate from the reusable compile report body.
func (c *Cmd) printProfile() error {
	fmt.Fprintln(os.Stdout, "forj build profile")
	return c.compileProfile.Print(os.Stdout, c.Top)
}

// buildBinaryWithCompileProfile compares uncached baseline and instrumented builds from the same source root.
func (c *Cmd) buildBinaryWithCompileProfile(root string, args []string) (string, error) {
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
	if err := c.runGoBuild(root, args, goBuildOptions{forceNoCache: true}); err != nil {
		return "", err
	}
	baselineTotalMS := time.Since(baselineStartedAt).Milliseconds()

	buildArgs := []string{"build", "-toolexec", exePath + " " + profileToolCommand, "-a"}
	buildArgs = append(buildArgs, args...)
	profiledStartedAt := time.Now()
	if err := c.runGoBuild(root, buildArgs[1:], goBuildOptions{
		extraEnv: []string{"FORJ_BUILD_PROFILE_LOG=" + logPath},
	}); err != nil {
		return "", err
	}
	profiledTotalMS := time.Since(profiledStartedAt).Milliseconds()

	report, err := compileprofile.Load(logPath)
	if err != nil {
		return "", err
	}
	report.NormalizeTimings(baselineTotalMS, profiledTotalMS)
	if err := report.AnnotateImportChains(root); err != nil {
		return "", fmt.Errorf("annotate compile profile import chains: %w", err)
	}
	c.compileProfile = report
	return "", nil
}

// runPlainGoBuild stages executable outputs so watchers only observe complete binaries.
func (c *Cmd) runPlainGoBuild(root string, args []string) (string, error) {
	c.lastBuildStatus = ""
	plan, err := planAtomicGoBuild(root, args)
	if err != nil {
		return "", err
	}
	if plan != nil {
		defer plan.cleanup()
		if err := c.runGoBuild(root, plan.args, goBuildOptions{allowRecovery: true}); err != nil {
			return "", err
		}
		if err := os.Chmod(plan.build, 0o755); err != nil {
			return "", fmt.Errorf("prepare built binary permissions: %w", err)
		}
		if err := os.Rename(plan.build, plan.final); err != nil {
			return "", fmt.Errorf("publish built binary: %w", err)
		}
		if err := writeBuildReadyStamp(plan.ready); err != nil {
			return "", fmt.Errorf("write build ready stamp: %w", err)
		}
		return c.lastBuildStatus, nil
	}
	if err := c.runGoBuild(root, args, goBuildOptions{allowRecovery: true}); err != nil {
		return "", err
	}
	return c.lastBuildStatus, nil
}

// atomicBuildPlan binds command arguments and publication paths for one safely staged binary build.
type atomicBuildPlan struct {
	args   []string
	final  string
	build  string
	ready  string
	legacy string
}

// planAtomicGoBuild stages watched binaries in a hidden unique file so dev never executes a partial file.
func planAtomicGoBuild(root string, args []string) (*atomicBuildPlan, error) {
	outIndex := outputArgIndex(args)
	if outIndex < 0 {
		return nil, nil
	}
	finalArg := outputPath(args[outIndex])
	if !atomicBuildOutputPath(rootedBuildPath(root, finalArg)) {
		return nil, nil
	}
	final := rootedBuildPath(root, finalArg)
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cacheDir := filepath.Join(dir, ".forj-build-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	buildName := uniqueBuildOutputName(filepath.Base(final))
	build := filepath.Join(cacheDir, buildName)
	buildArg := build
	if !filepath.IsAbs(finalArg) {
		buildArg = filepath.Join(filepath.Dir(finalArg), ".forj-build-cache", buildName)
	}
	atomicArgs := replaceBuildOutputArg(args, outIndex, buildArg)
	legacyBuild := filepath.Join(cacheDir, filepath.Base(final))
	return &atomicBuildPlan{
		args:   atomicArgs,
		final:  final,
		build:  build,
		ready:  buildReadyStampPath(final),
		legacy: legacyBuild,
	}, nil
}

// cleanup removes unpublished outputs without disturbing the last complete binary.
func (p *atomicBuildPlan) cleanup() {
	_ = os.Remove(p.build)
	_ = os.Remove(p.legacy)
}

// uniqueBuildOutputName avoids shared temp paths when dev and manual builds overlap.
func uniqueBuildOutputName(base string) string {
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		base = "app"
	}
	return fmt.Sprintf(".%s.%d.%d.build", base, os.Getpid(), atomicBuildOutputSequence.Add(1))
}

// buildReadyStampPath returns the watcher trigger written after a binary is fully published.
func buildReadyStampPath(final string) string {
	return filepath.Join(filepath.Dir(final), "."+filepath.Base(final)+".ready")
}

// writeBuildReadyStamp updates the watcher trigger after the executable is safe to launch.
func writeBuildReadyStamp(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	content := []byte(fmt.Sprintf("%d\n", time.Now().UnixNano()))
	return os.WriteFile(path, content, 0o644)
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

// replaceBuildOutputArg preserves the caller's flag form while redirecting publication through the staging path.
func replaceBuildOutputArg(args []string, outIndex int, output string) []string {
	out := append([]string(nil), args...)
	if strings.HasPrefix(out[outIndex], "-o=") {
		out[outIndex] = "-o=" + output
		return out
	}
	out[outIndex] = output
	return out
}

// runGoBuild executes Go from the selected project and optionally repairs missing module requirements once.
func (c *Cmd) runGoBuild(root string, args []string, opts goBuildOptions) error {
	buildArgs := append([]string{"build"}, args...)
	if opts.forceNoCache {
		buildArgs = append([]string{"build", "-a"}, args...)
	}
	cmd := exec.Command("go", buildArgs...)
	cmd.Dir = root
	if len(opts.extraEnv) > 0 {
		cmd.Env = append(os.Environ(), opts.extraEnv...)
	}
	if debugEnabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build: %w", err)
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
		if opts.allowRecovery {
			recovered, recoverErr := c.attemptMissingModuleRecovery(root, detail)
			if recoverErr != nil {
				return fmt.Errorf("recover missing build modules: %w", recoverErr)
			}
			if recovered {
				opts.allowRecovery = false
				return c.runGoBuild(root, args, opts)
			}
		}
		if detail != "" {
			return fmt.Errorf("go build: %w (%s)", err, detail)
		}
		return fmt.Errorf("go build: %w", err)
	}
	return nil
}

var missingModulePattern = regexp.MustCompile(`no required module provides package (\S+?)(?:;|\s|$)`)

// missingModulePackages extracts unique package paths so recovery installs only dependencies named by Go.
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

// attemptMissingModuleRecovery installs only dependencies named by Go's missing-module diagnostics.
func (c *Cmd) attemptMissingModuleRecovery(root string, detail string) (bool, error) {
	missingPkgs := missingModulePackages(detail)
	if len(missingPkgs) == 0 {
		return false, nil
	}
	if err := c.runGoGet(root, missingPkgs); err != nil {
		return false, err
	}
	c.lastBuildStatus = "synced deps, retried"
	return true, nil
}

// runGoGet updates the selected project's module files without relying on process working directory.
func (c *Cmd) runGoGet(root string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	if c.goGetFunc != nil {
		return c.goGetFunc(packages)
	}
	args := append([]string{"get"}, packages...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
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

// compilePackageName reads the compiler's package identity without depending on the rest of its unstable tool arguments.
func compilePackageName(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-p" {
			return args[i+1]
		}
	}
	return ""
}
