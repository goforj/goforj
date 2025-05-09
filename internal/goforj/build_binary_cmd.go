package goforj

import (
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"os"
	"os/exec"
	"runtime"
)

// BuildBinaryCmd is a command that builds the GoForge binary and installs it to the appropriate directory.
type BuildBinaryCmd struct {
	logger *logger.AppLogger
}

// NewBuildBinaryCmd creates a new instance of BuildBinaryCmd with the provided logger.
func NewBuildBinaryCmd(logger *logger.AppLogger) *BuildBinaryCmd {
	return &BuildBinaryCmd{
		logger: logger,
	}
}

// Run executes the build command to create the GoForge binary.
func (c *BuildBinaryCmd) Run() error {
	return BuildAndInstallGoForgeBinary()
}

// DetectBinDir detects the appropriate binary directory based on the operating system.
func DetectBinDir() (string, error) {
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

// BuildAndInstallGoForgeBinary builds the GoForge binary and installs it to the appropriate directory.
func BuildAndInstallGoForgeBinary() error {
	binDir, err := DetectBinDir()
	if err != nil {
		return err
	}

	fmt.Println("📦 Building GoForj binary...")

	buildCmd := exec.Command("go", "build", "-o", "./bin/goforj", "./main.go")
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
	copyCmd := exec.Command("cp", "./bin/goforj", binDir+"/goforj")
	copyCmd.Stdout = os.Stdout
	copyCmd.Stderr = os.Stderr
	if err := copyCmd.Run(); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make sure it's executable
	chmodCmd := exec.Command("chmod", "+x", binDir+"/goforj")
	chmodCmd.Stdout = os.Stdout
	chmodCmd.Stderr = os.Stderr
	if err := chmodCmd.Run(); err != nil {
		return fmt.Errorf("failed to chmod binary: %w", err)
	}

	fmt.Println("✅ GoForj binary installed successfully.")
	return nil
}
