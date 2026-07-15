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
	DisableFooter()
	EnableFooter()
	ResetFooterLine()
	SetStatusLine(string)
	MarkStatusDone()
	ClearStatusLine()
	HasStatusLine() bool
}

// devOutputSession names the terminal streams and lifecycle hooks that must change together when the TUI is enabled.
type devOutputSession struct {
	stdout   io.Writer
	stderr   io.Writer
	shutdown func()
	refresh  func()
}

// buildDevOutputSession selects plain terminal streams or a coordinated TUI output session.
func buildDevOutputSession(config *project.Config, requestRestart func(), requestRender func(), requestCommand func(devShellCommandRequest)) devOutputSession {
	if !term.IsTerminal(int(os.Stdout.Fd())) || strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" {
		return devOutputSession{
			stdout:   os.Stdout,
			stderr:   os.Stderr,
			shutdown: func() {},
			refresh:  func() {},
		}
	}
	return buildDevOutputSessionBubble(config, requestRestart, requestRender, requestCommand)
}

func disableDevFooter(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.DisableFooter()
	}
}

func enableDevFooter(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.EnableFooter()
	}
}

func resetDevFooterLine(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.ResetFooterLine()
	}
}

func setDevStatusLine(writer io.Writer, line string) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.SetStatusLine(line)
	}
}

func markDevStatusDone(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.MarkStatusDone()
	}
}

func clearDevStatusLine(writer io.Writer) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.ClearStatusLine()
	}
}

func hasDevStatusLine(writer io.Writer) bool {
	if controller := asDevOutputController(writer); controller != nil {
		return controller.HasStatusLine()
	}
	return false
}

func asDevOutputController(writer io.Writer) devOutputController {
	controller, _ := writer.(devOutputController)
	return controller
}
