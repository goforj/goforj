package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/goforj/goforj/internal/compileprofile"
	"github.com/goforj/web/webindex"
)

const profileToolCommand = "__forj_build_profile_exec"

// atomicBuildPublicationSequence avoids relying on host clock resolution for overlapping publication identity.
var atomicBuildPublicationSequence atomic.Uint64

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
		lock, err := webindex.AcquireArtifactPublicationLock(context.Background(), plan.build)
		if err != nil {
			return "", fmt.Errorf("lock build cache output: %w", err)
		}
		status, buildErr := c.runAtomicGoBuild(root, plan)
		releaseErr := lock.Release()
		if releaseErr != nil {
			releaseErr = fmt.Errorf("release build cache output lock: %w", releaseErr)
		}
		return status, errors.Join(buildErr, releaseErr)
	}
	if err := c.runGoBuild(root, args, goBuildOptions{allowRecovery: true}); err != nil {
		return "", err
	}
	return c.lastBuildStatus, nil
}

// runAtomicGoBuild keeps the stable linker output serialized until its exact completed inode has crossed the publication boundary.
func (c *Cmd) runAtomicGoBuild(root string, plan *atomicBuildPlan) (string, error) {
	defer plan.cleanup()
	if err := c.runGoBuild(root, plan.args, goBuildOptions{allowRecovery: true}); err != nil {
		return "", err
	}
	if err := os.Chmod(plan.build, 0o755); err != nil {
		return "", fmt.Errorf("prepare built binary permissions: %w", err)
	}
	if err := publishBuiltBinary(plan); err != nil {
		return "", err
	}
	plan.cleanupSupersededBuildOutput()
	if err := writeBuildReadyStamp(plan.ready); err != nil {
		return "", fmt.Errorf("write build ready stamp: %w", err)
	}
	return c.lastBuildStatus, nil
}

// atomicBuildPlan binds command arguments and publication paths for one safely staged binary build.
type atomicBuildPlan struct {
	args    []string
	final   string
	build   string
	publish string
	ready   string
	legacy  string
}

// planAtomicGoBuild keeps Go's output path stable for linker caching while reserving a unique final-publication path.
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
	buildName := filepath.Base(final)
	// Per-target directories keep the directory-scoped lock from serializing independent app builds.
	targetCacheDirName := buildName + ".target"
	targetCacheDir := filepath.Join(cacheDir, targetCacheDirName)
	if err := os.MkdirAll(targetCacheDir, 0o755); err != nil {
		return nil, err
	}
	build := filepath.Join(targetCacheDir, buildName)
	buildArg := build
	if !filepath.IsAbs(finalArg) {
		buildArg = filepath.Join(filepath.Dir(finalArg), ".forj-build-cache", targetCacheDirName, buildName)
	}
	atomicArgs := replaceBuildOutputArg(args, outIndex, buildArg)
	publish := filepath.Join(dir, uniqueBuildPublicationName(filepath.Base(final)))
	return &atomicBuildPlan{
		args:    atomicArgs,
		final:   final,
		build:   build,
		publish: publish,
		ready:   buildReadyStampPath(final),
		legacy:  filepath.Join(cacheDir, buildName),
	}, nil
}

// cleanup removes only this invocation's unpublished output and preserves Go's stable linker cache target.
func (p *atomicBuildPlan) cleanup() {
	_ = os.Remove(p.publish)
}

// cleanupSupersededBuildOutput removes the prior stable linker target after its replacement has been published.
func (p *atomicBuildPlan) cleanupSupersededBuildOutput() {
	if filepath.Clean(p.legacy) == filepath.Clean(p.build) {
		return
	}
	_ = os.Remove(p.legacy)
}

// uniqueBuildPublicationName prevents overlapping builders from sharing the path renamed over the live executable.
func uniqueBuildPublicationName(base string) string {
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		base = "app"
	}
	return fmt.Sprintf(".%s.%d.%d.publish", base, os.Getpid(), atomicBuildPublicationSequence.Add(1))
}

// publishBuiltBinary snapshots the serialized cache output before atomically replacing the executable observed by watchers.
func publishBuiltBinary(plan *atomicBuildPlan) error {
	return publishBuiltBinaryWithLink(plan, os.Link)
}

// publishBuiltBinaryWithLink falls back to a private complete copy on filesystems that do not support hard links.
func publishBuiltBinaryWithLink(plan *atomicBuildPlan, link func(string, string) error) error {
	if err := link(plan.build, plan.publish); err != nil {
		if copyErr := copyBuiltBinary(plan.build, plan.publish); copyErr != nil {
			return errors.Join(
				fmt.Errorf("link built binary for publication: %w", err),
				fmt.Errorf("copy built binary for publication: %w", copyErr),
			)
		}
	}
	if err := os.Rename(plan.publish, plan.final); err != nil {
		return fmt.Errorf("publish built binary: %w", err)
	}
	return nil
}

// copyBuiltBinary completes the fallback snapshot under its private name so a partial copy can never become the live executable.
func copyBuiltBinary(source string, destination string) (returnErr error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, output.Close())
		if returnErr != nil {
			_ = os.Remove(destination)
		}
	}()
	_, err = io.Copy(output, input)
	return err
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
