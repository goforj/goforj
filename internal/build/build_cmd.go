package build

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/logger"
)

// Cmd runs the forj build pipeline.
type Cmd struct {
	logger        *logger.AppLogger
	pipeline      Pipeline
	Timings       bool   `help:"Print per-step timings for generate, api index, and go build"`
	SkipWire      bool   `help:"Skip running wire before build" hidden:""`
	AutoRun       bool   `help:"Build binary so launching it with no args runs the app runtime command"`
	DefaultLaunch string `help:"Set compiled default command used when the built binary is launched without args"`
	EnvDefaults   string `help:"Compile unset-only environment defaults as comma-separated KEY=value pairs"`
	EnvOverrides  string `help:"Compile forced environment overrides as comma-separated KEY=value pairs"`

	// Profile flags.
	Profile bool `help:"Profile compile time for this build"`
	Top     int  `help:"Limit profile results" default:"12"`

	Root            string   `help:"Project root to build" default:"."`
	Args            []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to go build"`
	compileProfile  CompileProfileReport
	lastBuildStatus string
	goGetFunc       func([]string) error
}

func NewCmd(logger *logger.AppLogger, apiIndex *APIIndexRunner) *Cmd {
	return &Cmd{
		logger:   logger,
		pipeline: NewPipeline(logger, apiIndex),
	}
}

func (*Cmd) Signature() string {
	return `name:"build" help:"Run generate, API indexing, then go build" group:"build"`
}

func (c *Cmd) Run() error {
	if err := c.validateLaunchDefaults(); err != nil {
		return err
	}
	if err := c.pipeline.Run(c.Root, "build", Step{
		Name: "go build",
		Run:  c.buildBinary,
	}, RunOptions{Timings: c.Timings, SkipWire: c.SkipWire}); err != nil {
		return err
	}
	if c.Profile {
		return c.printProfile()
	}
	return nil
}

func (c *Cmd) buildBinary() (string, error) {
	args := c.buildArgs()
	if outIndex := outputArgIndex(args); outIndex >= 0 {
		if err := os.MkdirAll(filepath.Dir(outputPath(args[outIndex])), 0o755); err != nil {
			return "", err
		}
	}
	if c.Profile {
		return c.buildBinaryWithProfile(args)
	}
	return c.runPlainGoBuild(args)
}

func (c *Cmd) buildArgs() []string {
	defaultLaunch := c.effectiveDefaultLaunch()
	envDefaultsEncoded := c.encodedEnvDefaults()
	envOverridesEncoded := c.encodedEnvOverrides()
	modulePath := ""
	if defaultLaunch != "" || envDefaultsEncoded != "" || envOverridesEncoded != "" {
		modulePath = c.modulePath()
	}
	var extraLdflags []string
	if defaultLaunch != "" {
		extraLdflags = append(extraLdflags, c.defaultLaunchLdflags(modulePath, defaultLaunch))
	}
	if envDefaultsEncoded != "" {
		extraLdflags = append(extraLdflags, c.envDefaultsLdflags(modulePath, envDefaultsEncoded))
	}
	if envOverridesEncoded != "" {
		extraLdflags = append(extraLdflags, c.envOverridesLdflags(modulePath, envOverridesEncoded))
	}
	if len(c.Args) == 0 {
		target := activeApp()
		args := []string{"-o", filepath.ToSlash(filepath.Join(".", "bin", target.Name))}
		if len(extraLdflags) > 0 {
			args = append(args, "-ldflags", strings.Join(extraLdflags, " "))
		}
		return append(args, defaultBuildPackage(c.Root))
	}
	args := append([]string{}, c.Args...)
	if !hasGoBuildPackageArg(args) {
		args = append(args, defaultBuildPackage(c.Root))
	}
	if len(extraLdflags) == 0 {
		return args
	}
	return c.withExtraLdflags(args, extraLdflags...)
}

// hasGoBuildPackageArg reports whether pass-through go build args already name the package to build.
func hasGoBuildPackageArg(args []string) bool {
	flagsWithValue := map[string]struct{}{
		"-asmflags": {}, "-buildmode": {}, "-compiler": {}, "-gccgoflags": {}, "-gcflags": {},
		"-installsuffix": {}, "-ldflags": {}, "-mod": {}, "-modfile": {},
		"-o": {}, "-overlay": {}, "-p": {}, "-pkgdir": {}, "-tags": {}, "-toolexec": {},
	}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if arg == "--" {
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") {
				continue
			}
			// Some go build flags consume the next arg, so that value is not a package path.
			if _, ok := flagsWithValue[arg]; ok && i+1 < len(args) {
				i++
			}
			continue
		}
		return true
	}
	return false
}

// defaultBuildPackage keeps generated projects building the real app entrypoint instead of the framework root.
func defaultBuildPackage(root string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	target := activeApp()
	if packagePath := appPackageFromEntrypoint(target.Entrypoint); packagePath != "." {
		if info, err := os.Stat(filepath.Join(root, strings.TrimPrefix(packagePath, "./"))); err == nil && info.IsDir() {
			return packagePath
		}
	}
	if info, err := os.Stat(filepath.Join(root, "cmd", "app")); err == nil && info.IsDir() {
		return "./cmd/app"
	}
	return "."
}

func outputArgIndex(args []string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" {
			return i + 1
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-o=") {
			return i
		}
	}
	return -1
}

func outputPath(arg string) string {
	if strings.HasPrefix(arg, "-o=") {
		return strings.TrimPrefix(arg, "-o=")
	}
	return arg
}

func (c *Cmd) validateLaunchDefaults() error {
	if c.AutoRun && strings.TrimSpace(c.DefaultLaunch) != "" && strings.TrimSpace(c.DefaultLaunch) != "run" {
		return fmt.Errorf("--auto-run and --default-launch must agree; got default-launch=%q", c.DefaultLaunch)
	}
	if launch := strings.TrimSpace(c.DefaultLaunch); launch != "" && strings.ContainsAny(launch, " \t\r\n") {
		return fmt.Errorf("--default-launch must be a single command token, got %q", c.DefaultLaunch)
	}
	if _, err := parseEnvAssignments(c.EnvDefaults, "--env-defaults"); err != nil {
		return err
	}
	if _, err := parseEnvAssignments(c.EnvOverrides, "--env-overrides"); err != nil {
		return err
	}
	if modulePath := c.modulePath(); modulePath == "" && (c.effectiveDefaultLaunch() != "" || strings.TrimSpace(c.EnvDefaults) != "" || strings.TrimSpace(c.EnvOverrides) != "") {
		target := c.effectiveDefaultLaunch()
		if target != "" {
			return fmt.Errorf("could not resolve module path from %s/go.mod for default launch %q", strings.TrimSpace(c.Root), target)
		}
		if strings.TrimSpace(c.EnvDefaults) != "" {
			return fmt.Errorf("could not resolve module path from %s/go.mod for env defaults %q", strings.TrimSpace(c.Root), strings.TrimSpace(c.EnvDefaults))
		}
		return fmt.Errorf("could not resolve module path from %s/go.mod for env overrides %q", strings.TrimSpace(c.Root), strings.TrimSpace(c.EnvOverrides))
	}
	return nil
}

func (c *Cmd) effectiveDefaultLaunch() string {
	if launch := strings.TrimSpace(c.DefaultLaunch); launch != "" {
		return launch
	}
	if c.AutoRun {
		return "run"
	}
	return ""
}

func (c *Cmd) defaultLaunchLdflags(modulePath, defaultLaunch string) string {
	return fmt.Sprintf("-X %s/internal/cmd.DefaultLaunchCommand=%s", modulePath, defaultLaunch)
}

func (c *Cmd) encodedEnvDefaults() string {
	pairs, err := parseEnvAssignments(c.EnvDefaults, "--env-defaults")
	if err != nil || len(pairs) == 0 {
		return ""
	}
	raw := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		raw = append(raw, pair.key+"="+pair.value)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(raw, ",")))
}

func (c *Cmd) encodedEnvOverrides() string {
	pairs, err := parseEnvAssignments(c.EnvOverrides, "--env-overrides")
	if err != nil || len(pairs) == 0 {
		return ""
	}
	raw := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		raw = append(raw, pair.key+"="+pair.value)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(raw, ",")))
}

func (c *Cmd) envDefaultsLdflags(modulePath, encoded string) string {
	return fmt.Sprintf("-X %s/internal/cmd.CompiledEnvDefaultsBase64=%s", modulePath, encoded)
}

func (c *Cmd) envOverridesLdflags(modulePath, encoded string) string {
	return fmt.Sprintf("-X %s/internal/cmd.CompiledEnvOverridesBase64=%s", modulePath, encoded)
}

func (c *Cmd) withExtraLdflags(args []string, extras ...string) []string {
	ldflagsValue := strings.Join(extras, " ")
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-ldflags" {
			args[i+1] = strings.TrimSpace(args[i+1] + " " + ldflagsValue)
			return args
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-ldflags=") {
			current := strings.TrimPrefix(args[i], "-ldflags=")
			args[i] = "-ldflags=" + strings.TrimSpace(current+" "+ldflagsValue)
			return args
		}
	}
	return append([]string{"-ldflags", ldflagsValue}, args...)
}

type envDefaultPair struct {
	key   string
	value string
}

func parseEnvAssignments(raw string, flagName string) ([]envDefaultPair, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	entries := strings.Split(trimmed, ",")
	pairs := make([]envDefaultPair, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("%s contains an empty entry", flagName)
		}
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("%s entry %q must be KEY=value", flagName, entry)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s entry %q has an empty key", flagName, entry)
		}
		if strings.ContainsAny(key, " \t\r\n") {
			return nil, fmt.Errorf("%s key %q must not contain whitespace", flagName, key)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%s key %q is duplicated", flagName, key)
		}
		seen[key] = struct{}{}
		pairs = append(pairs, envDefaultPair{
			key:   key,
			value: strings.TrimSpace(value),
		})
	}
	return pairs, nil
}

func (c *Cmd) modulePath() string {
	root := c.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
