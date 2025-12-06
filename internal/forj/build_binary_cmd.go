package forj

import (
	"fmt"
	"github.com/goforj/forj/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildBinaryCmd is a command that builds the GoForj binary and installs it to the appropriate directory.
type BuildBinaryCmd struct {
	logger *logger.AppLogger
}

// NewBuildBinaryCmd creates a new instance of BuildBinaryCmd with the provided logger.
func NewBuildBinaryCmd(logger *logger.AppLogger) *BuildBinaryCmd {
	return &BuildBinaryCmd{
		logger: logger,
	}
}

// Run executes the build command to create the GoForj binary.
func (c *BuildBinaryCmd) Run() error {
	return BuildAndInstallGoForjBinary()
}

// BuildAndInstallGoForjBinary builds the GoForj binary and installs it to the appropriate directory.
func BuildAndInstallGoForjBinary() error {
	binDir, err := DetectBinDir()
	if err != nil {
		return err
	}

	fmt.Println("📦 Building GoForj binary...")

	buildCmd := exec.Command("go", "build", "-o", "./bin/", "./cmd/forj")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build: %w", err)
	}

	fmt.Println("📦 Installing GoForj binary to", binDir)

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	// Remove old binary if exists
	_ = os.Remove(binDir + "/goforj")

	// Move new binary
	copyCmd := exec.Command("cp", "./bin/forj", binDir+"/forj")
	copyCmd.Stdout = os.Stdout
	copyCmd.Stderr = os.Stderr
	if err := copyCmd.Run(); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make sure it's executable
	chmodCmd := exec.Command("chmod", "+x", binDir+"/forj")
	chmodCmd.Stdout = os.Stdout
	chmodCmd.Stderr = os.Stderr
	if err := chmodCmd.Run(); err != nil {
		return fmt.Errorf("failed to chmod binary: %w", err)
	}

	fmt.Println("✅ GoForj binary installed successfully.")
	return nil
}

// CheckPathForBinary checks if goforj already exists in PATH and returns its directory if found.
func CheckPathForBinary() (string, bool) {
	var whichCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		whichCmd = exec.Command("where", "forj.exe")
	} else {
		whichCmd = exec.Command("which", "forj")
	}

	output, err := whichCmd.Output()
	if err != nil {
		return "", false
	}

	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", false
	}

	dir := filepath.Dir(path)
	return dir, true
}

// DetectBinDir checks if the binary directory is already in PATH and returns it.
func DetectBinDir() (string, error) {
	if pathDir, found := CheckPathForBinary(); found {
		return pathDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/opt/homebrew/bin"); err == nil {
			return "/opt/homebrew/bin", nil
		}
		return homeDir + "/bin", nil
	} else {
		if _, err := os.Stat(homeDir + "/.local/bin"); err == nil {
			return homeDir + "/.local/bin", nil
		}
		return homeDir + "/bin", nil
	}
}
