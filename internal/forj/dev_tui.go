package forj

import (
	"io"
	"os"
	"strings"

	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

// devOutputController exposes TUI-only controls while allowing plain writers to remain valid session output.
type devOutputController interface {
	// DisableFooter defines the disable footer behavior required from implementations.
	DisableFooter()
	// EnableFooter defines the enable footer behavior required from implementations.
	EnableFooter()
	// ResetFooterLine defines the reset footer line behavior required from implementations.
	ResetFooterLine()
	// SetStatusLine defines the set status line behavior required from implementations.
	SetStatusLine(string)
	// MarkStatusDone defines the mark status done behavior required from implementations.
	MarkStatusDone()
	// ClearStatusLine defines the clear status line behavior required from implementations.
	ClearStatusLine()
	// HasStatusLine defines the has status line behavior required from implementations.
	HasStatusLine() bool
}

// devOutputSession names the terminal streams and lifecycle ownership that must change together when the TUI is enabled.
type devOutputSession struct {
	stdout           io.Writer
	stderr           io.Writer
	shutdown         func()
	refresh          func()
	restoresTerminal bool
}

// finishDevOutputSession falls back to a defensive reset only when the selected session did not capture and restore terminal state itself.
func finishDevOutputSession(session devOutputSession, fallbackRestore func()) {
	if session.shutdown != nil {
		session.shutdown()
		if session.restoresTerminal {
			return
		}
	}
	fallbackRestore()
}

// buildDevOutputSession selects plain terminal streams or a coordinated TUI output session.
func buildDevOutputSession(config *project.Config, requestRestart func(), requestRender func(), requestCommand func(devShellCommandRequest)) devOutputSession {
	if !term.IsTerminal(int(os.Stdout.Fd())) || strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" || devConfigUsesWatcherStdin(config) {
		return devOutputSession{
			stdout:   os.Stdout,
			stderr:   os.Stderr,
			shutdown: func() {},
			refresh:  func() {},
		}
	}
	return buildDevOutputSessionBubble(config, requestRestart, requestRender, requestCommand)
}

// devConfigUsesWatcherStdin keeps explicitly interactive children on the plain terminal because they cannot share input with Bubble Tea.
func devConfigUsesWatcherStdin(config *project.Config) bool {
	if config == nil {
		return false
	}
	for _, watch := range config.Dev.Watches {
		if watch.Stdin {
			return true
		}
		if strings.TrimSpace(watch.Watch) == "" {
			continue
		}
		options, err := parseLegacyDevWatchOptions(watch.Watch)
		if err == nil && options.stdin {
			return true
		}
	}
	return false
}

// disableDevFooter centralizes disable dev footer behavior so callers follow the same contract.
func disableDevFooter(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.DisableFooter()
	}
}

// enableDevFooter centralizes enable dev footer behavior so callers follow the same contract.
func enableDevFooter(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.EnableFooter()
	}
}

// resetDevFooterLine centralizes reset dev footer line behavior so callers follow the same contract.
func resetDevFooterLine(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.ResetFooterLine()
	}
}

// setDevStatusLine centralizes set dev status line behavior so callers follow the same contract.
func setDevStatusLine(writer io.Writer, line string) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.SetStatusLine(line)
	}
}

// markDevStatusDone centralizes mark dev status done behavior so callers follow the same contract.
func markDevStatusDone(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.MarkStatusDone()
	}
}

// clearDevStatusLine centralizes clear dev status line behavior so callers follow the same contract.
func clearDevStatusLine(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.ClearStatusLine()
	}
}

// hasDevStatusLine centralizes the has dev status line decision for its callers.
func hasDevStatusLine(writer io.Writer) bool {
	if controller := asDevOutputController(writer); controller != nil {
		return controller.HasStatusLine()
	}
	return false
}

// asDevOutputController centralizes as dev output controller behavior so callers follow the same contract.
func asDevOutputController(writer io.Writer) devOutputController {
	controller, _ := writer.(devOutputController)
	return controller
}
