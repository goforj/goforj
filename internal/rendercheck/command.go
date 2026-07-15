package rendercheck

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runForj executes the same binary that owns the render suite so acceptance checks cannot drift across versions.
func (worker renderComboWorker) runForj(arguments ...string) error {
	command := exec.Command(worker.forjExecutable, arguments...)
	command.Dir = worker.workspaceRoot
	command.Env = append(os.Environ(),
		"GOMODCACHE="+worker.moduleCache,
		"GOCACHE="+worker.buildCache,
	)

	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	label := "forj " + strings.Join(arguments, " ")
	if err := command.Run(); err != nil {
		return formatCommandFailure(label, err, stdout.String(), stderr.String())
	}
	if renderDebugEnabled() {
		if stdout.Len() > 0 {
			fmt.Printf("%s\n", strings.TrimSpace(stdout.String()))
		}
		if stderr.Len() > 0 {
			fmt.Printf("%s\n", strings.TrimSpace(stderr.String()))
		}
	}
	return nil
}
