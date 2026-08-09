package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/goforj/console"
	"golang.org/x/term"
)

const devSectionSeparatorRuleWidth = 5

// buildDevFooterSeparatorLine keeps the build dev footer separator line representation consistent.
func buildDevFooterSeparatorLine() string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}
	return buildDevFooterSeparatorLineAtWidth(width)
}

// buildDevFooterSeparatorLineAtWidth renders a subdued structural boundary at the model's current width.
func buildDevFooterSeparatorLineAtWidth(width int) string {
	if width <= 0 {
		width = 120
	}
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#E4E4E7", Dark: "#18181B"})
	return ruleStyle.Render(strings.Repeat("─", width))
}

// buildDevWatcherStopSeparatorLine keeps the transcript boundary compact once the full-width interactive footer is gone.
func buildDevWatcherStopSeparatorLine() string {
	borderColor := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	return lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", devSectionSeparatorRuleWidth*2))
}

// buildDevStartupSeparatorLine keeps the build dev startup separator line representation consistent.
func buildDevStartupSeparatorLine() string {
	success := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	return buildDevSectionSeparatorLine(success.Render(console.SuccessMark() + " Startup"))
}

// buildDevShutdownSeparatorLine keeps the build dev shutdown separator line representation consistent.
func buildDevShutdownSeparatorLine() string {
	transition := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9A3412", Dark: "#F59E0B"})
	return buildDevSectionSeparatorLine(transition.Render("•") + " Shutdown")
}

// buildDevSectionSeparatorLine keeps the build dev section separator line representation consistent.
func buildDevSectionSeparatorLine(label string) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}
	return buildDevSectionSeparatorLineAtWidth(label, width)
}

// buildDevSectionSeparatorLineAtWidth keeps compact-label and narrow-terminal behavior testable without depending on the test process TTY.
func buildDevSectionSeparatorLineAtWidth(label string, width int) string {
	if width <= 0 {
		width = 120
	}
	if width < 20 {
		width = 20
	}
	borderColor := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	if strings.TrimSpace(label) == "" {
		return lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", width))
	}
	ruleStyle := lipgloss.NewStyle().Foreground(borderColor)
	centerText := fmt.Sprintf(" %s ", strings.TrimSpace(label))
	centerWidth := lipgloss.Width(centerText)
	if width <= centerWidth {
		return centerText
	}
	sideWidth := devSectionSeparatorRuleWidth
	if available := (width - centerWidth) / 2; available < sideWidth {
		sideWidth = available
	}
	rule := ruleStyle.Render(strings.Repeat("─", sideWidth))
	return rule + centerText + rule
}

// buildDevFooterLine keeps the build dev footer line representation consistent.
func buildDevFooterLine(env map[string]string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(resolveAPIURL(env), resolveLighthouseUIURL(env), dbQueryLogging, appDebug)
}

// buildDevFooterLineWithURLs keeps the build dev footer line with urls representation consistent.
func buildDevFooterLineWithURLs(apiURL, lighthouseURL string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(apiURL, lighthouseURL, dbQueryLogging, appDebug)
}

// buildDevFooterLineWithState keeps the build dev footer line with state representation consistent.
func buildDevFooterLineWithState(apiURL, lighthouseURL string, dbQueryLogging bool, appDebug string) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}
	return buildDevFooterLineWithStateAtWidth(apiURL, lighthouseURL, dbQueryLogging, appDebug, width)
}

// buildDevFooterLineWithStateAtWidth composes status and action groups without allowing terminal wrapping.
func buildDevFooterLineWithStateAtWidth(apiURL, lighthouseURL string, dbQueryLogging bool, appDebug string, width int) string {
	if apiURL == "" && lighthouseURL == "" {
		return ""
	}
	statuses := make([]string, 0, 4)
	if lighthouseURL != "" {
		statuses = append(statuses, renderDevFooterBooleanStatus("Lighthouse", true))
	}
	if apiURL != "" {
		statuses = append(statuses, renderDevFooterBooleanStatus("API", true))
	}
	if lighthouseURL != "" || apiURL != "" {
		statuses = append(statuses, renderDevFooterBooleanStatus("Query", dbQueryLogging))
		statuses = append(statuses, renderDevFooterValueStatus("Debug", appDebug))
	}
	actions := []string{
		renderDevFooterShortcut("/", "Find"),
		renderDevFooterShortcut("x", "Command"),
		renderDevFooterShortcut("?", "Help"),
	}
	if width <= 0 {
		width = 120
	}
	booleanStatuses := statuses
	if len(booleanStatuses) > 0 {
		booleanStatuses = booleanStatuses[:len(booleanStatuses)-1]
	}
	help := actions[len(actions)-1:]
	for _, layout := range []struct {
		statuses []string
		actions  []string
		gap      int
	}{
		{statuses: statuses, actions: actions, gap: 5},
		{statuses: statuses, actions: actions, gap: 2},
		{statuses: statuses, actions: help, gap: 2},
		{statuses: booleanStatuses, actions: help, gap: 2},
		{statuses: booleanStatuses, gap: 2},
	} {
		if line, ok := layoutDevFooterGroups(layout.statuses, layout.actions, layout.gap, width); ok {
			return line
		}
	}
	return ansi.Truncate(strings.Join(booleanStatuses, "  "), width, "")
}

// layoutDevFooterGroups keeps statuses left-aligned and actions right-aligned when both fit on one row.
func layoutDevFooterGroups(statuses []string, actions []string, itemGap int, width int) (string, bool) {
	left := strings.Join(statuses, strings.Repeat(" ", itemGap))
	if len(actions) == 0 {
		if lipgloss.Width(left) <= width {
			return left, true
		}
		return "", false
	}
	right := strings.Join(actions, strings.Repeat(" ", itemGap))
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	const minimumGroupGap = 4
	if leftWidth+rightWidth+minimumGroupGap > width {
		return "", false
	}
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right, true
}

// renderDevFooterBooleanStatus pairs an enabled or disabled semantic indicator with a normal status label.
func renderDevFooterBooleanStatus(label string, active bool) string {
	labelStyle := devNormalForegroundStyle()
	indicator := "○"
	indicatorStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A1A1AA", Dark: "#52525B"})
	if active {
		indicator = "●"
		indicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"})
	}
	return indicatorStyle.Render(indicator) + " " + labelStyle.Render(label)
}

// renderDevFooterValueStatus preserves non-boolean runtime settings as a label and muted value.
func renderDevFooterValueStatus(label, value string) string {
	labelStyle := devNormalForegroundStyle()
	valueStyle := devMutedForegroundStyle()
	return labelStyle.Render(label) + " " + valueStyle.Render(strings.TrimSpace(value))
}

// renderDevFooterShortcut keeps the render dev footer shortcut representation consistent.
func renderDevFooterShortcut(key, label string) string {
	keyStyle := devInteractiveAccentStyle()
	labelStyle := devMutedForegroundStyle()
	return keyStyle.Render(key) + " " + labelStyle.Render(label)
}

// devInteractiveAccentStyle keeps keyboard destinations and actions visually related.
func devInteractiveAccentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
}

// devNormalForegroundStyle keeps persistent navigation labels quieter than semantic state.
func devNormalForegroundStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#3F3F46", Dark: "#E5E7EB"})
}

// devMutedForegroundStyle keeps secondary values and labels subordinate to interactive keys.
func devMutedForegroundStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
}

type devOverlaySpec struct {
	Title    string
	Hint     string
	Body     string
	MinWidth int
}

// buildDevOverlayRowsBox keeps the build dev overlay rows box representation consistent.
func buildDevOverlayRowsBox(spec devOverlaySpec, rows []string) string {
	spec.Body = strings.Join(rows, "\n")
	return buildDevOverlayBox(spec)
}

// buildDevResourceHeaderLine resolves the available terminal width for persistent resource navigation.
func buildDevResourceHeaderLine(tools []devToolLink) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}
	return buildDevResourceHeaderLineAtWidth(tools, width)
}

// buildDevResourceHeaderLineAtWidth composes complete keyboard destinations without wrapping the TUI.
func buildDevResourceHeaderLineAtWidth(tools []devToolLink, width int) string {
	if width <= 0 {
		width = 120
	}
	placeholderStyle := devMutedForegroundStyle()
	if len(tools) == 0 {
		return placeholderStyle.Render("loading…")
	}
	keyStyle := devInteractiveAccentStyle()
	labelStyle := devNormalForegroundStyle()
	items := make([]string, 0, min(len(tools), 9))
	for i, tool := range tools {
		if i >= 9 {
			break
		}
		items = append(items, keyStyle.Render(fmt.Sprintf("%d", i+1))+" "+labelStyle.Render(tool.Label))
	}
	for _, gap := range []int{5, 2} {
		line := strings.Join(items, strings.Repeat(" ", gap))
		if lipgloss.Width(line) <= width {
			return line
		}
	}
	ellipsis := placeholderStyle.Render("…")
	for visible := len(items) - 1; visible > 0; visible-- {
		line := strings.Join(items[:visible], "  ") + "  " + ellipsis
		if lipgloss.Width(line) <= width {
			return line
		}
	}
	return ansi.Truncate(items[0], width, "")
}

// buildDevHotkeyPanel keeps the build dev hotkey panel representation consistent.
func buildDevHotkeyPanel(tools []devToolLink, dbQueryLogging bool, appDebug string) []string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#27272A", Dark: "#F4F4F5"}).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#E5E7EB"})
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#71717A"}).Bold(true)
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#2F3136"})
	type hotkeyItem struct {
		key   string
		label string
	}
	sections := []struct {
		title string
		items []hotkeyItem
	}{
		{
			title: "TOGGLES",
			items: []hotkeyItem{
				{key: "q", label: renderDevFooterBooleanStatus("Query Logs", dbQueryLogging)},
				{key: "Shift+0-3", label: "Debug level"},
				{key: "f", label: "Component filters"},
			},
		},
		{
			title: "ACTIONS",
			items: []hotkeyItem{
				{key: "x", label: "Run command"},
				{key: "r", label: "Restart watchers"},
				{key: "c", label: "Clear screen"},
				{key: "/", label: "Find in transcript"},
				{key: "Tab / Shift+Tab", label: "Next / previous match"},
			},
		},
		{
			title: "VIEW",
			items: []hotkeyItem{
				{key: "↑/k", label: "Scroll up"},
				{key: "↓/j", label: "Scroll down"},
				{key: "PgUp/PgDn", label: "Page up / down"},
				{key: "l", label: "Jump to live tail"},
				{key: "G", label: "Jump to live tail (alt)"},
				{key: "g", label: "Jump to top"},
			},
		},
		{
			title: "LINKS",
			items: []hotkeyItem{},
		},
	}
	for i, tool := range tools {
		if i >= 9 {
			break
		}
		sections[3].items = append(sections[3].items, hotkeyItem{
			key:   fmt.Sprintf("%d", i+1),
			label: "Open " + tool.Label,
		})
	}
	sections = append(sections, struct {
		title string
		items []hotkeyItem
	}{
		title: "",
		items: []hotkeyItem{{key: "Esc/?", label: "Close"}},
	})
	maxKeyWidth := 0
	for _, section := range sections {
		for _, item := range section.items {
			if width := lipgloss.Width(item.key); width > maxKeyWidth {
				maxKeyWidth = width
			}
		}
	}
	blockLines := make([]string, 0, 16)
	for idx, section := range sections {
		if idx > 0 {
			blockLines = append(blockLines, ruleStyle.Render(strings.Repeat("─", 24)))
		}
		if section.title != "" {
			blockLines = append(blockLines, sectionStyle.Render(section.title))
		}
		for _, item := range section.items {
			blockLines = append(blockLines, renderDevHotkeyPanelItem(keyStyle, labelStyle, maxKeyWidth, item.key, item.label))
		}
	}
	body := strings.Join(blockLines, "\n")
	header := titleStyle.Render("Help")
	hint := renderDevFooterValueStatus("Debug", appDebug)
	headerGap := 34 - lipgloss.Width(header)
	if headerGap < 2 {
		headerGap = 2
	}
	content := lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Left, header, strings.Repeat(" ", headerGap), hint), body)
	box := buildDevOverlayBox(devOverlaySpec{
		Body: content,
	})
	return strings.Split(box, "\n")
}

// buildDevOverlayBox keeps the build dev overlay box representation consistent.
func buildDevOverlayBox(spec devOverlaySpec) string {
	content := spec.Body
	if strings.TrimSpace(spec.Title) != "" {
		titleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#27272A", Dark: "#F4F4F5"}).Bold(true)
		if strings.TrimSpace(content) != "" {
			content = lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(spec.Title), content)
		} else {
			content = titleStyle.Render(spec.Title)
		}
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}).
		Padding(1, 2)
	if spec.MinWidth > 0 {
		boxStyle = boxStyle.Width(spec.MinWidth)
	}
	box := boxStyle.Render(content)
	if strings.TrimSpace(spec.Hint) == "" {
		return box
	}
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	return lipgloss.JoinVertical(lipgloss.Center, box, "", hintStyle.Render(spec.Hint))
}

// buildDevFilterModalBox keeps the build dev filter modal box representation consistent.
func buildDevFilterModalBox(shown map[string]bool) string {
	keyStyle := devInteractiveAccentStyle()
	onStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"})
	offStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A1A1AA", Dark: "#52525B"})
	labelStyle := devNormalForegroundStyle()
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#2F3136"})
	const (
		filterKeyWidth   = 5
		filterStateWidth = 2
	)

	labelWidth := 0
	for _, component := range devComponentFilterOrder {
		if w := lipgloss.Width(component); w > labelWidth {
			labelWidth = w
		}
	}
	keyColumn := lipgloss.NewStyle().Width(filterKeyWidth)
	labelColumn := lipgloss.NewStyle().Width(labelWidth)
	stateColumn := lipgloss.NewStyle().Width(filterStateWidth)

	lines := make([]string, 0, len(devComponentFilterOrder)+3)
	for i, component := range devComponentFilterOrder {
		state := offStyle.Render("○")
		if shown[component] {
			state = onStyle.Render("●")
		}
		key := keyColumn.Render(keyStyle.Render(fmt.Sprintf("%d", i+1)))
		label := labelColumn.Render(labelStyle.Render(component))
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, stateColumn.Render(state), label))
	}
	lines = append(lines, ruleStyle.Render(strings.Repeat("─", filterKeyWidth+labelWidth+filterStateWidth)))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, keyColumn.Render(keyStyle.Render("a")), labelStyle.Render("Show all")))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, keyColumn.Render(keyStyle.Render("Esc")), labelStyle.Render("Close")))
	return buildDevOverlayRowsBox(devOverlaySpec{
		Title: "Component Filters",
		Hint:  "1–7 Toggle components",
	}, lines)
}

// buildDevHotkeyModalBox keeps the build dev hotkey modal box representation consistent.
func buildDevHotkeyModalBox(tools []devToolLink, dbQueryLogging bool, appDebug string) string {
	return strings.Join(buildDevHotkeyPanel(tools, dbQueryLogging, appDebug), "\n")
}

// buildDevCommandModalBox keeps the build dev command modal box representation consistent.
func buildDevCommandModalBox(commands []devAppCommandOption, selected int, args string, argsFocused bool, loadError string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#F4F4F5"})
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E5F7E8", Dark: "#1E2A21"}).
		Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F4F4F5"}).
		Bold(true)
	focusedInputStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E5F7E8", Dark: "#1E2A21"}).
		Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F4F4F5"}).
		Bold(true)
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#2F3136"})
	const (
		commandModalMinWidth     = 88
		commandModalNameWidth    = 24
		commandModalHelpWidth    = 46
		commandModalVisibleCount = 10
	)

	lines := []string{helpStyle.Render("App commands from " + activeDevAppBinaryPath() + " --help")}
	selectedAcceptsArgs := false
	if strings.TrimSpace(loadError) != "" {
		lines = append(lines, helpStyle.Render(loadError))
	} else if len(commands) == 0 {
		lines = append(lines, helpStyle.Render("No app commands available"))
	} else {
		if selected >= 0 && selected < len(commands) {
			selectedAcceptsArgs = commands[selected].AcceptsArgs
		}
		start := 0
		if selected > commandModalVisibleCount/2 {
			start = selected - commandModalVisibleCount/2
		}
		end := start + commandModalVisibleCount
		if end > len(commands) {
			end = len(commands)
			if end-commandModalVisibleCount > 0 {
				start = end - commandModalVisibleCount
			}
		}
		for i := start; i < end; i++ {
			command := commands[i]
			nameText := command.Name
			if lipgloss.Width(nameText) > commandModalNameWidth {
				nameText = truncateDevOverlayText(nameText, commandModalNameWidth)
			}
			helpText := truncateDevOverlayText(command.Help, commandModalHelpWidth)
			row := lipgloss.JoinHorizontal(
				lipgloss.Left,
				keyStyle.Render("›"),
				" ",
				nameStyle.Width(commandModalNameWidth).Render(nameText),
				"  ",
				helpStyle.Width(commandModalHelpWidth).Render(helpText),
			)
			if i == selected {
				row = selectedStyle.Render(stripANSIForSearch(row))
			}
			lines = append(lines, row)
		}
	}
	lines = append(lines, ruleStyle.Render(strings.Repeat("─", 32)))
	if selectedAcceptsArgs {
		argsValue := strings.TrimSpace(args)
		if argsValue == "" {
			argsValue = helpStyle.Render("--flag value")
		}
		argsLine := keyStyle.Render("Args") + "  " + nameStyle.Render(argsValue)
		if argsFocused {
			argsLine = focusedInputStyle.Render(stripANSIForSearch(argsLine))
		}
		lines = append(lines, argsLine)
	} else {
		lines = append(lines, helpStyle.Render("This command does not take args or flags"))
	}
	return buildDevOverlayRowsBox(devOverlaySpec{
		Title:    "Run Command",
		Hint:     commandModalHint(selectedAcceptsArgs, argsFocused),
		MinWidth: commandModalMinWidth,
	}, lines)
}

// commandModalHint centralizes command modal hint behavior so callers follow the same contract.
func commandModalHint(selectedAcceptsArgs bool, argsFocused bool) string {
	if selectedAcceptsArgs {
		if argsFocused {
			return "Shift+Tab Commands     Enter Run     Esc Close"
		}
		return "↑/↓ Select     Type Jump     Tab Args     Enter Run     Esc Close"
	}
	return "↑/↓ Select     Type Jump     Enter Run     Esc Close"
}

// truncateDevOverlayText centralizes truncate dev overlay text behavior so callers follow the same contract.
func truncateDevOverlayText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

// renderDevHotkeyPanelItem keeps the render dev hotkey panel item representation consistent.
func renderDevHotkeyPanelItem(keyStyle, labelStyle lipgloss.Style, keyWidth int, key, label string) string {
	if strings.TrimSpace(key) == "" && strings.TrimSpace(label) == "" {
		return ""
	}
	keyText := keyStyle.Render(key)
	padding := keyWidth - lipgloss.Width(key)
	if padding < 0 {
		padding = 0
	}
	return keyText + strings.Repeat(" ", padding+3) + labelStyle.Render(label)
}
