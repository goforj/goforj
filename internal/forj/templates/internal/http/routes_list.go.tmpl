package http

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/labstack/echo/v4"
	"hash/fnv"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

const maxMiddlewareColumnWidth = 50

type middlewareRenderConfig struct {
	useShortcodes bool
	nameToCode    map[string]string
}

// routeEntry represents a single route entry in the list.
type routeEntry struct {
	Path        string
	Handler     string
	Methods     []string
	Middlewares []string
}

// Styles for colorizing output
var (
	stylePath       = lipgloss.NewStyle().Foreground(lipgloss.Color("113")).Bold(true) // lime green
	styleGet        = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))              // green
	stylePost       = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))              // yellow
	styleDelete     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))              // red
	stylePatch      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))              // magenta
	stylePut        = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))              // blue
	styleHandler    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))             // white
	styleMiddleware = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))             // magenta
	styleCell       = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))              // default text
)

// getRoutesList builds a sorted slice of route entries from the registered groups.
func (s *Server) getRoutesList() []*routeEntry {
	grouped := map[string]*routeEntry{}

	for _, group := range s.groups {
		prefix := group.RoutePrefix()
		groupMW := middlewareFuncNames(group.Middlewares())

		for _, r := range group.Routes() {
			fullPath := prefix + r.Path()
			handlerName := qualifyHandler(runtime.FuncForPC(reflect.ValueOf(r.Handler()).Pointer()).Name())
			allMW := append(groupMW, middlewareFuncNames(r.Middlewares())...)
			key := fullPath + ":" + handlerName

			if _, ok := grouped[key]; !ok {
				grouped[key] = &routeEntry{
					Path:        fullPath,
					Handler:     handlerName,
					Methods:     []string{r.Method()},
					Middlewares: allMW,
				}
			} else {
				grouped[key].Methods = append(grouped[key].Methods, r.Method())
			}
		}
	}

	return sortRouteEntries(grouped)
}

// PrintRoutesList renders and prints the route table to stdout.
func (s *Server) PrintRoutesList() {
	entries := s.getRoutesList()
	useShortcodes := shouldUseMiddlewareShortcodes(entries)
	cfg := middlewareRenderConfig{}
	var legend map[string]string

	if useShortcodes {
		legend, cfg.nameToCode = buildMiddlewareShortcodes(entries)
		cfg.useShortcodes = true
	}

	rows := routeEntriesToRows(entries, cfg)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		PaddingLeft(1).
		PaddingRight(1).
		Foreground(lipgloss.Color("15"))
	cellStyle := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1)
	t := table.New().
		Border(lipgloss.ASCIIBorder()).
		BorderStyle(
			lipgloss.NewStyle().Foreground(
				lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"},
			),
		).
		BorderBottom(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		}).
		Headers("Path", "Methods", "Handler", "Middleware").
		Rows(rows...)

	borderColor := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	tableWidth := lipgloss.Width(t.Render())
	innerWidth := tableWidth - 2
	borderLine := borderStyle.Render("+" + strings.Repeat("-", innerWidth) + "+")

	pipeStyle := lipgloss.NewStyle().Foreground(borderColor)
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15"))

	pipe := pipeStyle.Render("|")
	label := labelStyle.Render(fmt.Sprintf(" API Routes › (%d)", len(entries)))

	fmt.Println(borderLine)
	if useShortcodes && len(legend) > 0 {
		maxCodeWidth := 0
		for code := range legend {
			if len(code) > maxCodeWidth {
				maxCodeWidth = len(code)
			}
		}

		fmt.Println(pipe + labelStyle.Render(" Middleware Legend"))
		for _, code := range sortedKeys(legend) {
			padded := fmt.Sprintf("%-*s", maxCodeWidth, code)
			fmt.Println(pipe + " " + styleMiddleware.Render(padded) + " \u00b7 " + legend[code])
		}
		fmt.Println(borderLine)
	}
	fmt.Println(pipe + label)
	fmt.Println(t.Render())
}

// routeEntriesToRows converts route entries into table rows for rendering.
func routeEntriesToRows(entries []*routeEntry, cfg middlewareRenderConfig) [][]string {
	var rows [][]string
	for _, entry := range entries {
		rows = append(rows, []string{
			stylePath.Render(entry.Path),
			colorizeMethods(entry.Methods),
			styleHandler.Render(entry.Handler),
			colorizeMiddleware(renderMiddlewareCell(entry.Middlewares, cfg)),
		})
	}
	return rows
}

// sortRouteEntries sorts grouped route entries by path and returns the slice.
func sortRouteEntries(grouped map[string]*routeEntry) []*routeEntry {
	var sorted []*routeEntry
	for _, entry := range grouped {
		sorted = append(sorted, entry)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	return sorted
}

// middlewareFuncNames extracts concise names of middleware functions.
func middlewareFuncNames(middlewares []echo.MiddlewareFunc) []string {
	var names []string
	for _, m := range middlewares {
		ptr := reflect.ValueOf(m).Pointer()
		fn := runtime.FuncForPC(ptr)
		if fn != nil {
			name := simplifyMiddlewareName(fn.Name())
			names = append(names, name)
		} else {
			names = append(names, "unknown")
		}
	}
	return names
}

// simplifyMiddlewareName shortens middleware function names for readability.
func simplifyMiddlewareName(name string) string {
	safe := filepath.ToSlash(name)
	safe = strings.TrimSuffix(safe, "-fm")
	safe = regexp.MustCompile(`\.func\d+$`).ReplaceAllString(safe, "")

	parts := strings.Split(safe, "/")
	last := parts[len(parts)-1]

	re := regexp.MustCompile(`\(\*([^)]+)\)\.([^.]+)$`)
	matches := re.FindStringSubmatch(last)
	if len(matches) == 3 {
		return fmt.Sprintf("%s.%s", matches[1], matches[2])
	}

	last = regexp.MustCompile(`\(\*[^)]+\)\.`).ReplaceAllString(last, "")
	last = strings.ReplaceAll(last, "ProvideRoutes.", "")
	last = strings.ReplaceAll(last, "ProvideAppRoutes.", "")

	dotParts := strings.Split(last, ".")
	if len(dotParts) > 1 {
		return fmt.Sprintf("%s.%s", dotParts[len(dotParts)-2], dotParts[len(dotParts)-1])
	}
	return last
}

// qualifyHandler formats the handler name to a more readable format.
func qualifyHandler(name string) string {
	safe := filepath.ToSlash(name)
	safe = strings.TrimSuffix(safe, "-fm")
	safe = regexp.MustCompile(`\.func\d+$`).ReplaceAllString(safe, "")

	lastDot := strings.LastIndex(safe, ".")
	if lastDot == -1 {
		return "handler.unknown"
	}
	method := safe[lastDot+1:]
	beforeMethod := safe[:lastDot]
	beforeMethod = regexp.MustCompile(`\(\*[^)]+\)`).ReplaceAllString(beforeMethod, "")
	beforeMethod = strings.TrimSuffix(beforeMethod, ".")

	parts := strings.Split(beforeMethod, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		pkg := strings.Trim(parts[i], ".")
		if !isGenericPackage(pkg) && pkg != "" {
			return fmt.Sprintf("%s.%s", pkg, method)
		}
	}
	return fmt.Sprintf("handler.%s", method)
}

// isGenericPackage checks if the package is a generic one that doesn't need to be displayed.
func isGenericPackage(pkg string) bool {
	switch pkg {
	case "internal", "http", "controllers", "handlers":
		return true
	default:
		return false
	}
}

// allHTTPMethods contains all standard HTTP methods.
var allHTTPMethods = []string{
	"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD",
	"CONNECT", "TRACE", "PROPFIND", "REPORT",
}

// unique returns a slice of unique strings from the input slice.
func unique(methods []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, m := range methods {
		if _, ok := seen[m]; !ok {
			seen[m] = struct{}{}
			out = append(out, m)
		}
	}
	return out
}

// normalizeMethods normalizes the list of HTTP methods to a string representation.
func normalizeMethods(methods []string) string {
	uniq := unique(methods)
	sort.Strings(uniq)

	all := append([]string(nil), allHTTPMethods...)
	sort.Strings(all)

	if len(uniq) == len(all) {
		for i := range uniq {
			if uniq[i] != all[i] {
				return strings.Join(uniq, ", ")
			}
		}
		return "ALL"
	}

	return strings.Join(uniq, ", ")
}

// colorizeMethods applies colors to HTTP methods.
func colorizeMethods(methods []string) string {
	normalized := normalizeMethods(methods)
	if normalized == "ALL" {
		return styleCell.Render(normalized)
	}

	parts := strings.Split(normalized, ",")
	colored := make([]string, 0, len(parts))
	for _, p := range parts {
		method := strings.TrimSpace(p)
		colored = append(colored, colorizeMethod(method))
	}
	return strings.Join(colored, ", ")
}

// colorizeMiddleware applies color to middleware strings.
func colorizeMiddleware(mw string) string {
	if mw == "" {
		return styleCell.Render("-")
	}
	return styleMiddleware.Render(mw)
}

// colorizeMethod applies colors to a single HTTP method.
func colorizeMethod(method string) string {
	switch method {
	case "GET":
		return styleGet.Render(method)
	case "POST":
		return stylePost.Render(method)
	case "DELETE":
		return styleDelete.Render(method)
	case "PATCH":
		return stylePatch.Render(method)
	case "PUT":
		return stylePut.Render(method)
	default:
		return styleCell.Render(method)
	}
}

// renderMiddlewareCell renders middleware names or shortcodes depending on configuration.
func renderMiddlewareCell(middlewares []string, cfg middlewareRenderConfig) string {
	if cfg.useShortcodes {
		var codes []string
		for _, mw := range middlewares {
			if code, ok := cfg.nameToCode[mw]; ok {
				codes = append(codes, code)
			} else {
				codes = append(codes, mw)
			}
		}
		return strings.Join(codes, ", ")
	}
	return strings.Join(middlewares, ", ")
}

// shouldUseMiddlewareShortcodes decides if middleware shortcodes should be used based on column width.
func shouldUseMiddlewareShortcodes(entries []*routeEntry) bool {
	for _, entry := range entries {
		width := lipgloss.Width(strings.Join(entry.Middlewares, ", "))
		if width > maxMiddlewareColumnWidth {
			return true
		}
	}
	return false
}

// buildMiddlewareShortcodes creates shortcode mappings for all middleware and returns code->name and name->code maps.
func buildMiddlewareShortcodes(entries []*routeEntry) (map[string]string, map[string]string) {
	codeToName := map[string]string{}
	nameToCode := map[string]string{}
	seen := map[string]struct{}{}

	for _, entry := range entries {
		for _, mw := range entry.Middlewares {
			if _, ok := seen[mw]; ok {
				continue
			}
			seen[mw] = struct{}{}
			base := friendlyMiddlewareCode(mw)
			offset := uint32(0)
			for {
				code := base
				if offset > 0 {
					code = fmt.Sprintf("%s-%02X", base, byte(fnvSuffix(mw, offset)))
				}
				if existing, ok := codeToName[code]; !ok || existing == mw {
					codeToName[code] = mw
					nameToCode[mw] = code
					break
				}
				offset++
			}
		}
	}

	return codeToName, nameToCode
}

// friendlyMiddlewareCode creates a mnemonic shortcode from package and handler parts.
func friendlyMiddlewareCode(name string) string {
	pkgPart, fnPart := splitMiddlewareName(name)
	pkgCode := uppercaseHint(pkgPart)
	fnCode := uppercaseHint(fnPart)

	if pkgCode == "" && fnCode == "" {
		return "MW"
	}
	if pkgCode == "" {
		return fnCode
	}
	if fnCode == "" {
		return pkgCode
	}
	return pkgCode + "." + fnCode
}

// uppercaseHint extracts uppercase letters, or falls back to the first letter if none.
func uppercaseHint(part string) string {
	if part == "" {
		return ""
	}
	var caps []rune
	for _, r := range part {
		if unicode.IsUpper(r) {
			caps = append(caps, r)
		}
	}
	if len(caps) > 0 {
		if len(caps) > 4 {
			caps = caps[:4]
		}
		return string(caps)
	}

	runes := []rune(part)
	return strings.ToUpper(string(runes[0]))
}

// fnvSuffix returns a stable byte derived from FNV-1a for collision disambiguation.
func fnvSuffix(name string, offset uint32) byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return byte(h.Sum32() + offset)
}

// splitMiddlewareName splits a middleware name into package and function parts.
func splitMiddlewareName(name string) (string, string) {
	parts := strings.Split(name, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2], parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return "", ""
}

// sortedKeys returns sorted keys from a map[string]string.
func sortedKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
