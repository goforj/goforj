package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/console"
	"golang.org/x/term"
)

func buildDevFooterSeparatorLine() string {
	return buildDevSectionSeparatorLine("")
}

func buildDevStartupSeparatorLine() string {
	success := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	return buildDevSectionSeparatorLine(success.Render(console.SuccessMark() + " Startup"))
}

func buildDevShutdownSeparatorLine() string {
	transition := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9A3412", Dark: "#F59E0B"})
	return buildDevSectionSeparatorLine(transition.Render("•") + " Shutdown")
}

func buildDevSectionSeparatorLine(label string) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
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
	left := 5
	if width-centerWidth-left < 0 {
		left = 0
	}
	right := width - centerWidth - left
	if right < 0 {
		right = 0
	}
	return ruleStyle.Render(strings.Repeat("─", left)) + centerText + ruleStyle.Render(strings.Repeat("─", right))
}

func buildDevFooterLine(env map[string]string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(resolveAPIURL(env), resolveLighthouseUIURL(env), dbQueryLogging, appDebug)
}

func buildDevFooterLineWithURLs(apiURL, lighthouseURL string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(apiURL, lighthouseURL, dbQueryLogging, appDebug)
}

func buildDevFooterLineWithState(apiURL, lighthouseURL string, dbQueryLogging bool, appDebug string) string {
	if apiURL == "" && lighthouseURL == "" {
		return ""
	}
	left := make([]string, 0, 4)
	if lighthouseURL != "" {
		left = append(left, renderDevFooterStatus("Lighthouse", "ON", true))
	}
	if apiURL != "" {
		left = append(left, renderDevFooterStatus("API", "ON", true))
	}
	if lighthouseURL != "" || apiURL != "" {
		queryState := "off"
		if dbQueryLogging {
			queryState = "on"
		}
		left = append(left, renderDevFooterStatus("Query", strings.ToUpper(queryState), dbQueryLogging))
		left = append(left, renderDevFooterStatus("Debug", appDebug, false))
	}
	right := []string{
		renderDevFooterShortcut("?", "Controls"),
		renderDevFooterShortcut("/", "Find"),
		renderDevFooterShortcut("x", "Command"),
		renderDevFooterShortcut("f", "Filters"),
		renderDevFooterShortcut("l", "Live"),
		renderDevFooterShortcut("r", "Restart"),
		renderDevFooterShortcut("c", "Clear"),
	}
	leftText := strings.Join(left, renderDevFooterDivider())
	rightText := strings.Join(right, "   ")
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return leftText + "    " + rightText
	}
	leftWidth := lipgloss.Width(leftText)
	rightWidth := lipgloss.Width(rightText)
	if leftWidth+rightWidth+4 >= width {
		return leftText + "    " + rightText
	}
	return leftText + strings.Repeat(" ", width-leftWidth-rightWidth) + rightText
}

func renderDevFooterDivider() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#A1A1AA", Dark: "#3F3F46"}).
		Render("  |  ")
}

func renderDevFooterStatus(label, state string, active bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#3F3F46", Dark: "#E5E7EB"})
	stateColor := lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#A1A1AA"}
	if active {
		stateColor = lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}
	}
	if strings.EqualFold(state, "OFF") {
		stateColor = lipgloss.AdaptiveColor{Light: "#9A3412", Dark: "#F97316"}
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		labelStyle.Render(label),
		" ",
		lipgloss.NewStyle().Foreground(stateColor).Bold(true).Render(state),
	)
}

func renderDevFooterShortcut(key, label string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#A1A1AA"})
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#27272A", Dark: "#F4F4F5"})
	return keyStyle.Render("["+key+"]") + " " + labelStyle.Render(label)
}

type devOverlaySpec struct {
	Title    string
	Hint     string
	Body     string
	MinWidth int
}

func buildDevOverlayRowsBox(spec devOverlaySpec, rows []string) string {
	spec.Body = strings.Join(rows, "\n")
	return buildDevOverlayBox(spec)
}

func buildDevResourceHeaderLine(tools []devToolLink) string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"}).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#27272A", Dark: "#F4F4F5"})
	placeholderStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#71717A"})

	items := []string{titleStyle.Render("Resources")}
	if len(tools) == 0 {
		items = append(items, placeholderStyle.Render("loading…"))
		return strings.Join(items, "   ")
	}
	for i, tool := range tools {
		if i >= 9 {
			break
		}
		items = append(items, lipgloss.JoinHorizontal(
			lipgloss.Left,
			keyStyle.Render("["+fmt.Sprintf("%d", i+1)+"]"),
			" ",
			labelStyle.Render(tool.Label),
		))
	}
	return strings.Join(items, "   ")
}

func buildDevHotkeyPanel(tools []devToolLink, dbQueryLogging bool, appDebug string) []string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#27272A", Dark: "#F4F4F5"}).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
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
				{key: "q", label: "Query Logs"},
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
		sections[2].items = append(sections[2].items, hotkeyItem{
			key:   fmt.Sprintf("%d", i+1),
			label: "Open " + tool.Label,
		})
	}
	sections = append(sections, struct {
		title string
		items []hotkeyItem
	}{
		title: "",
		items: []hotkeyItem{{key: "esc", label: "Close"}},
	})
	maxKeyWidth := 0
	for _, section := range sections {
		for _, item := range section.items {
			if width := lipgloss.Width("[" + item.key + "]"); width > maxKeyWidth {
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
	header := titleStyle.Render("Hotkeys")
	hint := mutedStyle.Render("debug:" + appDebug)
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

func buildDevFilterModalBox(shown map[string]bool) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
	onStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#166534", Dark: "#7CFC93"}).Bold(true)
	offStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9A3412", Dark: "#F97316"}).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#E5E7EB"})
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#2F3136"})
	const (
		filterKeyWidth   = 6
		filterStateWidth = 3
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
		state := offStyle.Render("OFF")
		if shown[component] {
			state = onStyle.Render("ON")
		}
		key := keyColumn.Render(keyStyle.Render("[" + fmt.Sprintf("%d", i+1) + "]"))
		label := labelColumn.Render(labelStyle.Render(component))
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, label, "   ", stateColumn.Render(state)))
	}
	lines = append(lines, ruleStyle.Render(strings.Repeat("─", filterKeyWidth+labelWidth+6+filterStateWidth)))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, keyColumn.Render(keyStyle.Render("[a]")), labelStyle.Render("Show all")))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, keyColumn.Render(keyStyle.Render("[esc]")), labelStyle.Render("Close")))
	return buildDevOverlayRowsBox(devOverlaySpec{
		Title: "Component Filters",
		Hint:  "Toggle components with [1-7]",
	}, lines)
}

func buildDevHotkeyModalBox(tools []devToolLink, dbQueryLogging bool, appDebug string) string {
	panel := strings.Join(buildDevHotkeyPanel(tools, dbQueryLogging, appDebug), "\n")
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#71717A"})
	return lipgloss.JoinVertical(
		lipgloss.Center,
		panel,
		"",
		hintStyle.Render("Press Esc or [?] to close"),
	)
}

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

	lines := []string{helpStyle.Render("App commands from ./bin/app --help")}
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

func commandModalHint(selectedAcceptsArgs bool, argsFocused bool) string {
	if selectedAcceptsArgs {
		if argsFocused {
			return "Use Shift+Tab to return to the command list, Enter to run"
		}
		return "Use ↑/↓ to select, type to jump, Tab for args, Enter to run"
	}
	return "Use ↑/↓ to select, type to jump, Enter to run"
}

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

func renderDevHotkeyPanelItem(keyStyle, labelStyle lipgloss.Style, keyWidth int, key, label string) string {
	if strings.TrimSpace(key) == "" && strings.TrimSpace(label) == "" {
		return ""
	}
	keyText := keyStyle.Render("[" + key + "]")
	padding := keyWidth - lipgloss.Width("["+key+"]")
	if padding < 0 {
		padding = 0
	}
	return keyText + strings.Repeat(" ", padding+3) + labelStyle.Render(label)
}
