package forj

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	charmansi "github.com/charmbracelet/x/ansi"
	"github.com/goforj/goforj/project"
)

var ansiCode = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiCode.ReplaceAllString(s, "")
}

func TestBuildDevFooterLine(t *testing.T) {
	line := stripANSI(buildDevFooterLine(map[string]string{
		"APP_URL":            "http://127.0.0.1:3000",
		"LIGHTHOUSE_URL":     "ws://127.0.0.1:3000/lighthouse/ws/agent",
		"LIGHTHOUSE_ENABLED": "true",
	}))
	if !strings.Contains(line, "Lighthouse") || !strings.Contains(line, "API") {
		t.Fatalf("expected hotkey help in footer line: %q", line)
	}
	if !strings.Contains(line, "[?] Controls") || !strings.Contains(line, "[r] Restart") || !strings.Contains(line, "[c] Clear") {
		t.Fatalf("expected action hotkeys in footer line: %q", line)
	}
	if !strings.Contains(line, "[/] Find") {
		t.Fatalf("expected find shortcut in footer line: %q", line)
	}
	if !strings.Contains(line, "[x] Command") {
		t.Fatalf("expected command shortcut in footer line: %q", line)
	}
}

// TestDevConfigUsesWatcherStdinKeepsInteractiveChildrenOffTheTUI verifies both supported config shapes retain direct terminal ownership.
func TestDevConfigUsesWatcherStdinKeepsInteractiveChildrenOffTheTUI(t *testing.T) {
	tests := []struct {
		name   string
		config *project.Config
		want   bool
	}{
		{name: "nil config"},
		{name: "noninteractive", config: &project.Config{Dev: project.DevConfig{Watches: []project.DevWatch{{Exec: "wails dev"}}}}},
		{name: "structured stdin", config: &project.Config{Dev: project.DevConfig{Watches: []project.DevWatch{{Exec: "go run .", Stdin: true}}}}, want: true},
		{name: "legacy stdin", config: &project.Config{Dev: project.DevConfig{Watches: []project.DevWatch{{Watch: "-file .go -stdin", Exec: "go run ."}}}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := devConfigUsesWatcherStdin(test.config); got != test.want {
				t.Fatalf("devConfigUsesWatcherStdin() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestFinishDevOutputSessionRestoresTerminalExactlyOnce protects the handoff from adding duplicate reset newlines after the TUI exits.
func TestFinishDevOutputSessionRestoresTerminalExactlyOnce(t *testing.T) {
	tests := []struct {
		name                    string
		hasShutdown             bool
		restoresTerminal        bool
		wantShutdownCalls       int
		wantFallbackRestoration int
	}{
		{name: "before session selection", wantFallbackRestoration: 1},
		{name: "missing terminal owner", restoresTerminal: true, wantFallbackRestoration: 1},
		{name: "plain session", hasShutdown: true, wantShutdownCalls: 1, wantFallbackRestoration: 1},
		{name: "bubble session", hasShutdown: true, restoresTerminal: true, wantShutdownCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shutdownCalls := 0
			fallbackRestorations := 0
			ownedRestorations := 0
			session := devOutputSession{restoresTerminal: test.restoresTerminal}
			if test.hasShutdown {
				session.shutdown = func() {
					shutdownCalls++
					if test.restoresTerminal {
						ownedRestorations++
					}
				}
			}

			finishDevOutputSession(session, func() {
				fallbackRestorations++
			})

			if shutdownCalls != test.wantShutdownCalls {
				t.Fatalf("shutdown calls = %d, want %d", shutdownCalls, test.wantShutdownCalls)
			}
			if fallbackRestorations != test.wantFallbackRestoration {
				t.Fatalf("fallback restorations = %d, want %d", fallbackRestorations, test.wantFallbackRestoration)
			}
			if got := ownedRestorations + fallbackRestorations; got != 1 {
				t.Fatalf("terminal restorations = %d, want exactly 1", got)
			}
		})
	}
}

// TestDevTerminalModeResetSequenceDoesNotAdvanceTheCursor keeps post-TUI output adjacent to the bootstrap transcript.
func TestDevTerminalModeResetSequenceDoesNotAdvanceTheCursor(t *testing.T) {
	if strings.Contains(devTerminalModeResetSequence, "\n") {
		t.Fatalf("terminal reset sequence advances to another row: %q", devTerminalModeResetSequence)
	}
	if !strings.HasSuffix(devTerminalModeResetSequence, "\r\x1b[2K") {
		t.Fatalf("terminal reset sequence does not clear the restored cursor row: %q", devTerminalModeResetSequence)
	}
}

func TestBuildDevFooterLineWithURLs(t *testing.T) {
	line := stripANSI(buildDevFooterLineWithState("http://localhost:3000", "http://localhost:3000/lighthouse", true, "2"))
	if strings.Contains(line, "\n") {
		t.Fatalf("expected single-line footer, got multiline output: %q", line)
	}
	if !strings.Contains(line, "Lighthouse") || !strings.Contains(line, "API") {
		t.Fatalf("expected compact hotkeys in line: %q", line)
	}
	if !strings.Contains(line, "Query") || !strings.Contains(line, "ON") || !strings.Contains(line, "Debug") || !strings.Contains(line, "2") {
		t.Fatalf("expected state pills in line: %q", line)
	}
	if strings.Contains(line, "[ Lighthouse") || strings.Contains(line, "Lighthouse  ON ]") {
		t.Fatalf("expected flatter footer status format, got: %q", line)
	}
	if !strings.Contains(line, "[?] Controls") || !strings.Contains(line, "[x] Command") || !strings.Contains(line, "[r] Restart") || !strings.Contains(line, "[c] Clear") {
		t.Fatalf("expected env/restart hotkeys in line: %q", line)
	}
}

// TestBuildDevSectionSeparatorLineUsesCompactBalancedRules prevents wide terminals from dominating the development transcript.
func TestBuildDevSectionSeparatorLineUsesCompactBalancedRules(t *testing.T) {
	tests := []struct {
		name  string
		label string
		width int
		want  string
	}{
		{name: "wide terminal", label: "Start", width: 120, want: "───── Start ─────"},
		{name: "unavailable terminal", label: "Start", want: "───── Start ─────"},
		{name: "narrow terminal", label: "A label wider than the minimum terminal", width: 12, want: " A label wider than the minimum terminal "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line := stripANSI(buildDevSectionSeparatorLineAtWidth(test.label, test.width))
			if line != test.want {
				t.Fatalf("separator = %q, want %q", line, test.want)
			}
		})
	}
}

// TestBuildDevSectionSeparatorLineKeepsFullWidthForUnlabeledBoundaries preserves structural TUI edges that rely on terminal width.
func TestBuildDevSectionSeparatorLineKeepsFullWidthForUnlabeledBoundaries(t *testing.T) {
	line := stripANSI(buildDevSectionSeparatorLineAtWidth("", 48))
	if got := charmansi.StringWidth(line); got != 48 {
		t.Fatalf("unlabeled separator width = %d, want 48: %q", got, line)
	}
}

// TestBuildDevWatcherStopSeparatorLineStaysCompact prevents shutdown from restoring a terminal-width rule above the watcher summary.
func TestBuildDevWatcherStopSeparatorLineStaysCompact(t *testing.T) {
	line := stripANSI(buildDevWatcherStopSeparatorLine())
	if want := strings.Repeat("─", devSectionSeparatorRuleWidth*2); line != want {
		t.Fatalf("watcher stop separator = %q, want %q", line, want)
	}
}

// TestDevBubbleModelResetFooterLine keeps the rendered footer synchronized with the writer's refreshed default.
func TestDevBubbleModelResetFooterLine(t *testing.T) {
	model := devBubbleModel{footerLine: "temporary"}
	next, _ := model.Update(devResetFooterMsg{line: "default"})

	if got := next.(devBubbleModel).footerLine; got != "default" {
		t.Fatalf("reset footer line = %q, want %q", got, "default")
	}
}

func TestBuildDevHotkeyPanel(t *testing.T) {
	panel := strings.Join(buildDevHotkeyPanel([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
		{Label: "Lighthouse", URL: "http://localhost:3000/lighthouse"},
	}, false, "0"), "\n")
	panel = stripANSI(panel)
	for _, want := range []string{"Hotkeys", "TOGGLES", "[q]", "Query Logs", "[Shift+0-3]", "Debug level", "ACTIONS", "[x]", "Run command", "[r]", "Restart watchers", "[/]", "Find in transcript", "LINKS", "[1]", "Open App", "[2]", "Open Lighthouse", "[esc]", "Close"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("expected %q in hotkey panel:\n%s", want, panel)
		}
	}
}

func TestBuildDevCommandModalBox(t *testing.T) {
	box := stripANSI(buildDevCommandModalBox([]devAppCommandOption{
		{Name: "route:list", Help: "List HTTP routes", AcceptsArgs: true},
		{Name: "migrate", Help: "Run database migration"},
	}, 0, "--json", true, ""))
	for _, want := range []string{"Run Command", "App commands from ./bin/app --help", "route:list", "List HTTP routes", "Args", "--json"} {
		if !strings.Contains(box, want) {
			t.Fatalf("expected %q in command modal box:\n%s", want, box)
		}
	}
}

func TestBuildDevCommandModalBoxUsesActiveApp(t *testing.T) {
	t.Setenv("FORJ_APP", "customer-portal")
	box := stripANSI(buildDevCommandModalBox([]devAppCommandOption{
		{Name: "route:list", Help: "List HTTP routes"},
	}, 0, "", false, ""))
	if !strings.Contains(box, "App commands from ./bin/customer-portal --help") {
		t.Fatalf("expected app command source in command modal box:\n%s", box)
	}
}

func TestBuildDevCommandModalBoxWithoutArgs(t *testing.T) {
	box := stripANSI(buildDevCommandModalBox([]devAppCommandOption{
		{Name: "route:list", Help: "List HTTP routes"},
	}, 0, "", false, ""))
	if strings.Contains(box, "Args") {
		t.Fatalf("expected no args prompt for no-args command:\n%s", box)
	}
	if !strings.Contains(box, "This command does not take args or flags") {
		t.Fatalf("expected no-args guidance in command modal:\n%s", box)
	}
}

func TestParseDevAppHelpCommands(t *testing.T) {
	help := "\n› App\n\n  \x1b[1;38;5;113mroute:list\x1b[0m        List HTTP routes\n  \x1b[1;38;5;113mmigrate\x1b[0m           Run database migration\n"
	commands := parseDevAppHelpCommands(help)
	if len(commands) != 2 {
		t.Fatalf("expected 2 parsed commands, got %#v", commands)
	}
	if commands[0].Name != "route:list" || commands[0].Help != "List HTTP routes" {
		t.Fatalf("unexpected first command: %#v", commands[0])
	}
}

func TestParseDevAppCommandAcceptsArgs(t *testing.T) {
	withArgs := "\n› Run database migration\n\n  --dry-run  Preview work without writing\n"
	if !parseDevAppCommandAcceptsArgs(withArgs) {
		t.Fatal("expected command help with flags to accept args")
	}
	withoutArgs := "\n› List HTTP routes\n\n"
	if parseDevAppCommandAcceptsArgs(withoutArgs) {
		t.Fatal("expected command help without positional/flag rows to reject args")
	}
}

func TestBuildDevHotkeyModalBoxIncludesCloseHint(t *testing.T) {
	box := stripANSI(buildDevHotkeyModalBox([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
	}, false, "0"))
	if !strings.Contains(box, "Press Esc or [?] to close") {
		t.Fatalf("expected close hint in hotkey modal box:\n%s", box)
	}
}

func TestWriteDevCommandLineUsesSectionSeparator(t *testing.T) {
	var buf bytes.Buffer
	writeDevCommandLine(&buf, "Running ./bin/app about")
	got := stripANSI(buf.String())
	if !strings.Contains(got, "Running ./bin/app about") {
		t.Fatalf("expected command heading in output, got %q", got)
	}
	if !strings.Contains(got, "─") {
		t.Fatalf("expected section separator framing, got %q", got)
	}
	if strings.Contains(got, "· Running ./bin/app about") {
		t.Fatalf("expected framed command line instead of action bullet, got %q", got)
	}
}

func TestWriteDevCommandBoundaryUsesPlainSeparator(t *testing.T) {
	var buf bytes.Buffer
	writeDevCommandBoundary(&buf)
	got := stripANSI(buf.String())
	if !strings.Contains(got, "─") {
		t.Fatalf("expected section separator framing, got %q", got)
	}
	if strings.Contains(got, "Running ./bin/app") {
		t.Fatalf("expected plain separator without repeated command label, got %q", got)
	}
}

func TestBuildDevHotkeyPanelAlignsKeys(t *testing.T) {
	panel := buildDevHotkeyPanel([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
		{Label: "Lighthouse", URL: "http://localhost:3000/lighthouse"},
	}, false, "0")
	if len(panel) < 6 {
		t.Fatalf("expected boxed panel output, got %d lines", len(panel))
	}
	lines := make([]string, 0, 6)
	for _, line := range panel {
		plain := stripANSI(line)
		if strings.Contains(plain, "Query Logs") || strings.Contains(plain, "Debug level") || strings.Contains(plain, "Restart watchers") || strings.Contains(plain, "Clear screen") || strings.Contains(plain, "Open Lighthouse") || strings.Contains(plain, "Open App") || strings.Contains(plain, "Close") {
			lines = append(lines, plain)
		}
	}
	if len(lines) < 5 {
		t.Fatalf("expected grouped panel rows, got %d", len(lines))
	}
	labelPos := -1
	for _, line := range lines {
		idx := strings.Index(line, "]")
		if idx < 0 {
			t.Fatalf("expected key block in line %q", line)
		}
		labelIdx := idx + 1
		for labelIdx < len(line) && line[labelIdx] == ' ' {
			labelIdx++
		}
		if labelIdx-idx < 3 {
			t.Fatalf("expected readable gap after key block in line %q", line)
		}
		if labelPos == -1 {
			labelPos = labelIdx
			continue
		}
		if labelIdx != labelPos {
			t.Fatalf("expected aligned label starts at %d, got %d in line %q", labelPos, labelIdx, line)
		}
	}
}

func TestRenderDevBubbleOverlay(t *testing.T) {
	base := "line1................\nleft-background-right\nline3................\nline4"
	overlay := "box1\nbox2"
	got := renderDevBubbleOverlay(base, overlay, 20, 10)
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line4") {
		t.Fatalf("expected non-overlay base lines preserved in overlay output, got %q", got)
	}
	if !strings.Contains(got, "box1") || !strings.Contains(got, "box2") {
		t.Fatalf("expected centered overlay content in output, got %q", got)
	}
	if !strings.Contains(got, "left-") || !strings.Contains(got, "-right") {
		t.Fatalf("expected overlapped row to preserve surrounding base content, got %q", got)
	}
}

func TestSplitANSITail(t *testing.T) {
	head, tail := splitANSITail("ok \x1b[31mred\x1b[0m done")
	if head != "ok \x1b[31mred\x1b[0m done" || tail != "" {
		t.Fatalf("unexpected split complete sequence: head=%q tail=%q", head, tail)
	}

	head, tail = splitANSITail("ok \x1b[31")
	if head != "ok " || tail != "\x1b[31" {
		t.Fatalf("unexpected split partial sequence: head=%q tail=%q", head, tail)
	}
}

func TestSanitizeCSIPreservesColorCodes(t *testing.T) {
	in := "ok \x1b[32mgreen\x1b[0m \x1b[2Kgone"
	out := sanitizeCSI(in)
	if !strings.Contains(out, "\x1b[32mgreen\x1b[0m") {
		t.Fatalf("expected SGR color codes to be preserved, got: %q", out)
	}
	if strings.Contains(out, "\x1b[2K") {
		t.Fatalf("expected cursor control codes to be stripped, got: %q", out)
	}
}

func TestSanitizeCSIStripsOSCAndSingleEscapes(t *testing.T) {
	in := "left\x1b]8;;http://localhost:3000\x07link\x1b]8;;\x07right\x1b7save\x1b8restore"
	out := sanitizeCSI(in)
	if strings.Contains(out, "\x1b]8;") {
		t.Fatalf("expected OSC hyperlink sequences to be stripped, got: %q", out)
	}
	if strings.Contains(out, "\x1b7") || strings.Contains(out, "\x1b8") {
		t.Fatalf("expected single-character escape sequences to be stripped, got: %q", out)
	}
	if !strings.Contains(out, "leftlinkrightsaverestore") {
		t.Fatalf("expected visible content to remain after stripping, got: %q", out)
	}
}

func TestUpdateEnvKey(t *testing.T) {
	in := "APP_DEBUG=1\nDB_QUERY_LOGGING=false\n"
	out := updateEnvKey(in, "APP_DEBUG", "2")
	if !strings.Contains(out, "APP_DEBUG=2") {
		t.Fatalf("expected APP_DEBUG updated: %q", out)
	}
	out = updateEnvKey(out, "DB_QUERY_LOGGING", "true")
	if !strings.Contains(out, "DB_QUERY_LOGGING=true") {
		t.Fatalf("expected DB_QUERY_LOGGING updated: %q", out)
	}
}

func TestReadEnvKey(t *testing.T) {
	in := "# comment\nexport APP_DEBUG=3\nDB_QUERY_LOGGING=1\n"
	if got := readEnvKey(in, "APP_DEBUG"); got != "3" {
		t.Fatalf("expected APP_DEBUG=3, got %q", got)
	}
	if got := readEnvKey(in, "DB_QUERY_LOGGING"); got != "1" {
		t.Fatalf("expected DB_QUERY_LOGGING=1, got %q", got)
	}
}

func TestBuildDevResourceHeaderLine(t *testing.T) {
	line := stripANSI(buildDevResourceHeaderLine([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
		{Label: "Lighthouse", URL: "http://localhost:3000/lighthouse"},
		{Label: "Mailpit", URL: "http://localhost:8025"},
	}))
	for _, want := range []string{"Resources", "[1] App", "[2] Lighthouse", "[3] Mailpit"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in resource header: %q", want, line)
		}
	}
}

func TestBuildDevResourceHeaderLinePlaceholder(t *testing.T) {
	line := stripANSI(buildDevResourceHeaderLine(nil))
	for _, want := range []string{"Resources", "loading"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in resource header placeholder: %q", want, line)
		}
	}
}

func TestDevBubbleModelCurrentOverlay(t *testing.T) {
	m := devBubbleModel{
		tools: []devToolLink{
			{Label: "App", URL: "http://localhost:3000"},
		},
		componentShown: defaultDevComponentShown(),
		appDebug:       "0",
	}
	if got := m.currentOverlay(); got != "" {
		t.Fatalf("expected no overlay by default, got %q", stripANSI(got))
	}

	m.helpVisible = true
	if got := stripANSI(m.currentOverlay()); !strings.Contains(got, "Hotkeys") {
		t.Fatalf("expected hotkey overlay when help is visible, got %q", got)
	}

	m.commandVisible = true
	if got := stripANSI(m.currentOverlay()); !strings.Contains(got, "Run Command") {
		t.Fatalf("expected command overlay to take precedence, got %q", got)
	}

	m.commandVisible = false
	m.filterVisible = true
	if got := stripANSI(m.currentOverlay()); !strings.Contains(got, "Component Filters") {
		t.Fatalf("expected filter overlay to take precedence, got %q", got)
	}
}

func TestDevTranscriptComponent(t *testing.T) {
	line := "\x1b[90m19:27:32.402\x1b[0m \x1b[34mHTTP\x1b[0m HTTP Request"
	if got := devTranscriptComponent(line); got != "HTTP" {
		t.Fatalf("expected HTTP component, got %q", got)
	}
	multiAppLine := "19:27:32.402 billing HTTP HTTP Request"
	if got := devTranscriptComponent(multiAppLine); got != "HTTP" {
		t.Fatalf("expected HTTP component from multi-app line, got %q", got)
	}
	if got := devTranscriptComponent("Starting Run App - ./bin/app run"); got != "" {
		t.Fatalf("expected no component for orchestration line, got %q", got)
	}
}

func TestFilterDevTranscriptLines(t *testing.T) {
	lines := []string{
		"19:27:32.402 HTTP HTTP Request",
		"19:27:32.403 Jobs Job processed",
		"Starting Run App - ./bin/app run",
	}
	filtered := filterDevTranscriptLines(lines, map[string]bool{
		"HTTP":      false,
		"Jobs":      true,
		"Scheduler": true,
		"System":    true,
		"Error":     true,
		"Database":  true,
		"Cache":     true,
	})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 visible lines after HTTP filter, got %d: %#v", len(filtered), filtered)
	}
	if strings.Contains(filtered[0], "HTTP") {
		t.Fatalf("expected HTTP line to be filtered out, got %#v", filtered)
	}
}

func TestWrapDevTranscriptLines(t *testing.T) {
	lines := wrapDevTranscriptLines([]string{
		"HTTP Request → latency=12.43ms · method=GET · status=200 · uri=/api/v1/monitoring/monitors/7/dashboard?range=1h&ts=1778627475662",
	}, 48)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped transcript lines, got %d: %#v", len(lines), lines)
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if got := len([]rune(stripANSI(line))); got > 60 {
			t.Fatalf("expected wrapped line to stay bounded, got width-ish %d for %q", got, line)
		}
	}
}

func TestDevBubbleModelScrollAndFollow(t *testing.T) {
	m := devBubbleModel{
		width:          120,
		height:         12,
		footerEnabled:  true,
		followMode:     true,
		componentShown: defaultDevComponentShown(),
	}
	for i := 0; i < 20; i++ {
		m.lines = append(m.lines, "19:27:32.402 HTTP HTTP Request")
	}
	m.scrollUp(1)
	if m.followMode {
		t.Fatal("expected scroll up to disable follow mode")
	}
	if m.viewportTop <= 0 {
		t.Fatalf("expected viewport top to move into transcript, got %d", m.viewportTop)
	}
	m.scrollToBottom()
	if !m.followMode {
		t.Fatal("expected scrollToBottom to re-enable follow mode")
	}
}

func TestDevBubbleModelSearchMatchesAndJump(t *testing.T) {
	m := devBubbleModel{
		width:          120,
		height:         12,
		footerEnabled:  true,
		followMode:     true,
		componentShown: defaultDevComponentShown(),
		lines: []string{
			"19:27:32.402 HTTP HTTP Request one",
			"19:27:32.403 Jobs Job processed",
			"19:27:32.404 HTTP HTTP Request two",
		},
		searchQuery: "request",
		searchIndex: -1,
	}
	m.updateSearchMatches()
	if len(m.searchMatches) != 2 {
		t.Fatalf("expected 2 search matches, got %d", len(m.searchMatches))
	}
	m.jumpSearch(1)
	if m.searchIndex != 1 {
		t.Fatalf("expected first search jump to land on index 1, got %d", m.searchIndex)
	}
	if m.followMode {
		t.Fatal("expected search jump to disable follow mode")
	}
}

func TestDevBubbleModelContextStatusLineShowsFindHints(t *testing.T) {
	m := devBubbleModel{
		searchMode:  true,
		searchQuery: "heartbeat",
	}
	if got := m.contextStatusLine(); !strings.Contains(got, "[Enter apply]") || !strings.Contains(got, "[Esc clear]") {
		t.Fatalf("expected find entry hints in status line, got %q", got)
	}

	m.searchMode = false
	m.searchMatches = []int{2, 5, 9}
	m.searchIndex = 1
	if got := m.contextStatusLine(); !strings.Contains(got, "[Tab/Shift+Tab]") || !strings.Contains(got, "[Esc]") || !strings.Contains(got, "(2/3)") {
		t.Fatalf("expected active find hints in status line, got %q", got)
	}
}

func TestDevBubbleModelEscClearsActiveFindState(t *testing.T) {
	m := devBubbleModel{
		searchQuery:   "request",
		searchMatches: []int{1, 4},
		searchIndex:   0,
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(devBubbleModel)
	if got.searchMode {
		t.Fatal("expected esc to leave find mode inactive")
	}
	if got.searchQuery != "" || len(got.searchMatches) != 0 || got.searchIndex != -1 {
		t.Fatalf("expected esc to clear find state, got query=%q matches=%v index=%d", got.searchQuery, got.searchMatches, got.searchIndex)
	}
}

func TestDevBubbleModelHelpHotkeyExecutesAndDismisses(t *testing.T) {
	restarts := 0
	m := devBubbleModel{
		helpVisible:    true,
		requestRestart: func() { restarts++ },
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := next.(devBubbleModel)
	if got.helpVisible {
		t.Fatal("expected help overlay to dismiss after restart hotkey")
	}
	if restarts != 1 {
		t.Fatalf("expected restart hotkey to fire once, got %d", restarts)
	}
	if len(got.lines) == 0 || !strings.Contains(stripANSI(got.lines[len(got.lines)-1]), "Restart requested") {
		t.Fatalf("expected restart notice appended to transcript, got %#v", got.lines)
	}
}

func TestDevBubbleModelHelpFindHotkeyOpensFindAndDismisses(t *testing.T) {
	m := devBubbleModel{
		helpVisible: true,
		searchQuery: "old",
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	got := next.(devBubbleModel)
	if got.helpVisible {
		t.Fatal("expected help overlay to dismiss after find hotkey")
	}
	if !got.searchMode {
		t.Fatal("expected find mode to activate from help overlay")
	}
	if got.searchQuery != "" || len(got.searchMatches) != 0 || got.searchIndex != -1 {
		t.Fatalf("expected fresh find state, got query=%q matches=%v index=%d", got.searchQuery, got.searchMatches, got.searchIndex)
	}
}

func TestDevBubbleModelCommandHotkeyOpensPalette(t *testing.T) {
	m := devBubbleModel{
		helpVisible: true,
		commands: []devAppCommandOption{
			{Name: "route:list", Help: "List HTTP routes"},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	got := next.(devBubbleModel)
	if got.helpVisible {
		t.Fatal("expected help overlay to dismiss after command hotkey")
	}
	if !got.commandVisible {
		t.Fatal("expected command palette to open")
	}
}

func TestDevBubbleModelCommandEnterExecutesSelection(t *testing.T) {
	requests := []devShellCommandRequest{}
	m := devBubbleModel{
		commandVisible: true,
		commands: []devAppCommandOption{
			{Name: "route:list", Help: "List HTTP routes", AcceptsArgs: true},
		},
		commandArgs: "--json",
		requestCommand: func(req devShellCommandRequest) {
			requests = append(requests, req)
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(devBubbleModel)
	if got.commandVisible {
		t.Fatal("expected command palette to close after executing")
	}
	if len(requests) != 1 {
		t.Fatalf("expected one command request, got %#v", requests)
	}
	if requests[0].ShellCommand != "./bin/app route:list --json" {
		t.Fatalf("unexpected shell command: %#v", requests[0])
	}
}

func TestDevBubbleModelCommandEnterUsesActiveApp(t *testing.T) {
	t.Setenv("FORJ_APP", "customer-portal")
	requests := []devShellCommandRequest{}
	m := devBubbleModel{
		commandVisible: true,
		commands: []devAppCommandOption{
			{Name: "route:list", Help: "List HTTP routes", AcceptsArgs: true},
		},
		requestCommand: func(req devShellCommandRequest) {
			requests = append(requests, req)
		},
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(devBubbleModel).commandVisible {
		t.Fatal("expected command palette to close after executing")
	}
	if len(requests) != 1 {
		t.Fatalf("expected one command request, got %#v", requests)
	}
	if requests[0].ShellCommand != "./bin/customer-portal route:list" {
		t.Fatalf("unexpected shell command: %#v", requests[0])
	}
}

func TestDevBubbleModelCommandTypingJumpsSelectionByPrefix(t *testing.T) {
	m := devBubbleModel{
		commandVisible: true,
		commands: []devAppCommandOption{
			{Name: "make:controller", Help: "Make controller"},
			{Name: "route:list", Help: "List HTTP routes"},
			{Name: "test:openapi", Help: "Run OpenAPI checks"},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := next.(devBubbleModel)
	if got.commandIndex != 1 {
		t.Fatalf("expected typing to jump to route:list, got index %d", got.commandIndex)
	}
	if got.commandArgs != "" {
		t.Fatalf("expected prefix jump not to write args, got %q", got.commandArgs)
	}
}

func TestDevBubbleModelCommandTabFocusesArgsInput(t *testing.T) {
	m := devBubbleModel{
		commandVisible: true,
		commands: []devAppCommandOption{
			{Name: "route:list", Help: "List HTTP routes", AcceptsArgs: true},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(devBubbleModel)
	if !got.commandArgsFocus {
		t.Fatal("expected tab to move focus to args input")
	}
}

func TestDevBubbleModelCommandTypingInArgsFocusWritesArgs(t *testing.T) {
	m := devBubbleModel{
		commandVisible:   true,
		commandArgsFocus: true,
		commands: []devAppCommandOption{
			{Name: "route:list", Help: "List HTTP routes", AcceptsArgs: true},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	got := next.(devBubbleModel)
	if got.commandArgs != "-" {
		t.Fatalf("expected args-focused typing to write args, got %q", got.commandArgs)
	}
}

func TestDecorateDevSearchMatches(t *testing.T) {
	lines := []string{"first", "second", "third"}
	got := decorateDevSearchMatches(lines, 10, []int{11, 12}, 1)
	if got[0] != "first" {
		t.Fatalf("expected non-match line unchanged, got %q", got[0])
	}
	if got[1] == "second" {
		t.Fatal("expected non-current match line to be decorated")
	}
	if got[2] == "third" {
		t.Fatal("expected current match line to be decorated")
	}
	if got[1] == got[2] {
		t.Fatal("expected current match decoration to differ from non-current match decoration")
	}
}

func TestApplyDevLineHighlightPersistsAcrossANSIResets(t *testing.T) {
	line := "\x1b[34mHTTP\x1b[0m Request \x1b[90m→\x1b[0m status=\x1b[32m200\x1b[0m"
	got := applyDevLineHighlight(line, "\x1b[48;5;236m")
	if strings.Count(got, "\x1b[48;5;236m") < 3 {
		t.Fatalf("expected highlight prefix to be reapplied across ANSI resets, got %q", got)
	}
}

func TestWrapDevTranscriptLineIndentsMetadataContinuation(t *testing.T) {
	line := "19:19:04.585 HTTP         HTTP Request → latency=7.57ms · method=GET · status=200 · uri=/api/v1/monitoring/heartbeats?limit=12&ids=2%2C9%2C15%2C3%2C6%2C1%2C12%2C11%2C5%2C8%2C16%2C14%2C7%2C10%2C13%2C4"
	lines := wrapDevTranscriptLine(line, 96)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped metadata continuation, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[1], "  · ") {
		t.Fatalf("expected continuation to be indented under metadata region, got %q", stripANSI(lines[1]))
	}
}

func TestWrapDevTranscriptLineIndentsColoredMetadataContinuation(t *testing.T) {
	line := "19:19:04.585 HTTP         HTTP Request \x1b[90m→\x1b[0m latency=7.57ms · method=GET · status=200 · uri=/api/v1/monitoring/heartbeats?limit=12&ids=2%2C9%2C15%2C3%2C6%2C1%2C12%2C11%2C5%2C8%2C16%2C14%2C7%2C10%2C13%2C4"
	lines := wrapDevTranscriptLine(line, 96)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped colored metadata continuation, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[1], "  · ") {
		t.Fatalf("expected colored continuation to be indented under metadata region, got %q", stripANSI(lines[1]))
	}
}

func TestWrapDevTranscriptLinePacksMetadataFieldsWithoutDoubleBullet(t *testing.T) {
	line := "01:40:42.465 HTTP         monitoring summary loaded → checks_last_hour=889 · incidents_open=0 · inspect_id=dihcdjpx8z489 · maintenance_active=false · monitors_down=0 · monitors_paused=2 · monitors_pending=0 · monitors_total=16 · monitors_up=14 · source=http"
	lines := wrapDevTranscriptLine(line, 120)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped metadata continuation, got %d lines", len(lines))
	}
	if strings.Contains(stripANSI(lines[1]), "· ·") {
		t.Fatalf("expected continuation without duplicated bullet, got %q", stripANSI(lines[1]))
	}
	if !strings.Contains(stripANSI(lines[1]), "monitors_down=0 · monitors_paused=2") {
		t.Fatalf("expected continuation to keep multiple kv pairs together, got %q", stripANSI(lines[1]))
	}
}

func TestWrapDevTranscriptLinePacksColoredMetadataFieldsWithoutDoubleBullet(t *testing.T) {
	line := "01:40:42.465 HTTP         monitoring summary loaded \x1b[90m→\x1b[0m checks_last_hour=1227 \x1b[90m·\x1b[0m incidents_open=0 \x1b[90m·\x1b[0m inspect_id=dihcrirsq8tck \x1b[90m·\x1b[0m maintenance_active=false \x1b[90m·\x1b[0m monitors_down=0 \x1b[90m·\x1b[0m monitors_paused=2 \x1b[90m·\x1b[0m monitors_pending=0 \x1b[90m·\x1b[0m monitors_total=16 \x1b[90m·\x1b[0m monitors_up=14 \x1b[90m·\x1b[0m source=http"
	lines := wrapDevTranscriptLine(line, 120)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped metadata continuation, got %d lines", len(lines))
	}
	if strings.Contains(stripANSI(lines[1]), "· ·") {
		t.Fatalf("expected colored continuation without duplicated bullet, got %q", stripANSI(lines[1]))
	}
	if !strings.Contains(stripANSI(lines[1]), "monitors_down=0 · monitors_paused=2") {
		t.Fatalf("expected colored continuation to keep multiple kv pairs together, got %q", stripANSI(lines[1]))
	}
	for _, wrapped := range lines {
		if got := charmansi.StringWidth(stripANSI(wrapped)); got > 120 {
			t.Fatalf("expected colored wrapped line to stay within visible width, got %d for %q", got, stripANSI(wrapped))
		}
	}
}

func TestWrapDevTranscriptLineKeepsVisibleWidthBoundedWithColoredValues(t *testing.T) {
	line := "02:13:31.583 HTTP         HTTP Request \x1b[90m→\x1b[0m latency=\x1b[36m4.66ms\x1b[0m \x1b[90m·\x1b[0m method=\x1b[37mGET\x1b[0m \x1b[90m·\x1b[0m status=\x1b[37m200\x1b[0m \x1b[90m·\x1b[0m uri=\x1b[37m/api/v1/monitoring/summary\x1b[0m"
	lines := wrapDevTranscriptLine(line, 72)
	if len(lines) < 2 {
		t.Fatalf("expected colored metadata line to wrap, got %d lines", len(lines))
	}
	for _, wrapped := range lines {
		if got := charmansi.StringWidth(stripANSI(wrapped)); got > 72 {
			t.Fatalf("expected colored wrapped line to stay within visible width, got %d for %q", got, stripANSI(wrapped))
		}
	}
}
