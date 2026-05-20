package forj

import (
	"io"
	"os"
	"strings"

	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

type devOutputController interface {
	DisableFooter()
	EnableFooter()
	SetFooterLine(string)
	ResetFooterLine()
	SetStatusLine(string)
	MarkStatusDone()
	ClearStatusLine()
	HasStatusLine() bool
}

func buildDevOutputWriters(config *project.Config, requestRestart func(), requestRender func(), requestCommand func(devShellCommandRequest)) (io.Writer, io.Writer, func(), func()) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, os.Stderr, func() {}, func() {}
	}
	if strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" {
		return os.Stdout, os.Stderr, func() {}, func() {}
	}
	return buildDevOutputWritersBubble(config, requestRestart, requestRender, requestCommand)
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

func setDevFooterLine(writer io.Writer, line string) {
	if controller := asDevOutputController(writer); controller != nil {
		controller.SetFooterLine(line)
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
