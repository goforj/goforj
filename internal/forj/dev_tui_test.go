package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.Contains(line, "[?] Controls") || !strings.Contains(line, "[r] Restart") || !strings.Contains(line, "[c] Clear") {
		t.Fatalf("expected env/restart hotkeys in line: %q", line)
	}
}

func TestBuildDevHotkeyPanel(t *testing.T) {
	panel := strings.Join(buildDevHotkeyPanel([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
		{Label: "Lighthouse", URL: "http://localhost:3000/lighthouse"},
	}, false, "0"), "\n")
	panel = stripANSI(panel)
	for _, want := range []string{"Hotkeys", "TOGGLES", "[q]", "Query Logs", "[Shift+0-3]", "Debug level", "ACTIONS", "[r]", "Restart watchers", "[/]", "Find in transcript", "LINKS", "[1]", "Open App", "[2]", "Open Lighthouse", "[esc]", "Close"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("expected %q in hotkey panel:\n%s", want, panel)
		}
	}
}

func TestRenderDevHotkeyModalIncludesCloseHint(t *testing.T) {
	view := stripANSI(renderDevHotkeyModal([]devToolLink{
		{Label: "App", URL: "http://localhost:3000"},
		{Label: "Lighthouse", URL: "http://localhost:3000/lighthouse"},
	}, false, "0"))
	for _, want := range []string{"Hotkeys", "Press Esc or [?] to close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in modal view:\n%s", want, view)
		}
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

func TestRenderDevFilterModal(t *testing.T) {
	view := stripANSI(renderDevFilterModal(map[string]bool{
		"HTTP":      true,
		"Jobs":      false,
		"Scheduler": true,
		"System":    true,
		"Error":     true,
		"Database":  true,
		"Cache":     true,
	}))
	for _, want := range []string{"Component Filters", "[1]", "HTTP", "ON", "[2]", "Jobs", "OFF", "[a]", "Show all", "[esc]", "Close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in filter modal:\n%s", want, view)
		}
	}
}

func TestDevTranscriptComponent(t *testing.T) {
	line := "\x1b[90m19:27:32.402\x1b[0m \x1b[34mHTTP\x1b[0m HTTP Request"
	if got := devTranscriptComponent(line); got != "HTTP" {
		t.Fatalf("expected HTTP component, got %q", got)
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
	if got := m.contextStatusLine(); !strings.Contains(got, "[Tab next]") || !strings.Contains(got, "[Shift+Tab prev]") || !strings.Contains(got, "(2/3)") {
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

func TestDevFooterControllerBareDigitHotkeyDoesNotMutateEnvOrRestart(t *testing.T) {
	t.Setenv("APP_URL", "http://localhost:3000")

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_DEBUG=3\nDB_QUERY_LOGGING=false\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	var buf bytes.Buffer
	writer := newDevFooterWriter(&buf, "---", "footer")
	restarted := false
	controller := &devFooterController{
		writer:         writer,
		apiURL:         "http://localhost:3000",
		requestRestart: func() { restarted = true },
		appDebug:       "3",
	}

	controller.handleHotkeyByte('1')

	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(content), "APP_DEBUG=3") {
		t.Fatalf("expected naked digit hotkey not to mutate APP_DEBUG, got: %q", string(content))
	}
	if restarted {
		t.Fatal("expected naked digit hotkey not to restart watchers")
	}
}

func TestDevFooterControllerShiftDigitHotkeySetsDebugLevel(t *testing.T) {
	t.Setenv("APP_URL", "http://localhost:3000")

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_DEBUG=1\nDB_QUERY_LOGGING=false\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	var buf bytes.Buffer
	writer := newDevFooterWriter(&buf, "---", "footer")
	restartCount := 0
	controller := &devFooterController{
		writer:         writer,
		apiURL:         "http://localhost:3000",
		requestRestart: func() { restartCount++ },
		appDebug:       "1",
	}

	controller.handleHotkeyByte('#')

	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(content), "APP_DEBUG=3") {
		t.Fatalf("expected shifted digit hotkey to update APP_DEBUG, got: %q", string(content))
	}
	if restartCount != 1 {
		t.Fatalf("expected one watcher restart, got %d", restartCount)
	}
}

func TestDevFooterControllerQuestionHotkeyTogglesPanel(t *testing.T) {
	var buf bytes.Buffer
	writer := newDevFooterWriter(&buf, "---", "footer")
	controller := &devFooterController{
		writer:         writer,
		apiURL:         "http://localhost:3000",
		lighthouseURL:  "http://localhost:3000/lighthouse",
		dbQueryLogging: false,
		appDebug:       "0",
	}

	controller.handleHotkeyByte('?')
	if !controller.helpVisible {
		t.Fatal("expected question hotkey to show help modal")
	}

	controller.handleHotkeyByte('?')
	if controller.helpVisible {
		t.Fatal("expected second question hotkey to hide help modal")
	}
}

func TestDevFooterControllerCloseHotkeyPanel(t *testing.T) {
	var buf bytes.Buffer
	writer := newDevFooterWriter(&buf, "---", "footer")
	controller := &devFooterController{
		writer:         writer,
		apiURL:         "http://localhost:3000",
		lighthouseURL:  "http://localhost:3000/lighthouse",
		dbQueryLogging: false,
		appDebug:       "0",
	}

	controller.handleHotkeyByte('?')
	if !controller.helpVisible {
		t.Fatal("expected panel to open")
	}
	if !controller.closeHotkeyPanel() {
		t.Fatal("expected closeHotkeyPanel to report true when panel is open")
	}
	if controller.helpVisible {
		t.Fatal("expected closeHotkeyPanel to hide the panel")
	}
	if controller.closeHotkeyPanel() {
		t.Fatal("expected closeHotkeyPanel to report false when panel is already closed")
	}
}
