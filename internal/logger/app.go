package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/goforj/str"
	"github.com/rs/zerolog"
)

// AppLogger represents a debug logger.
type AppLogger struct {
	debugLogger *zerolog.Logger // The debugLogger.
	infoLogger  *zerolog.Logger // The infoLogger.
	debugLevel  int             // The debugLogger level. 1,2,3
}

// logConfig stores resolved logging configuration values.
type logConfig struct {
	appEnv     string
	appMode    string
	format     string
	prefix     string
	showCaller bool
}

const (
	logFormatEnv  = "APP_LOG_FORMAT"
	logFormatJSON = "json"
)

// NewAppLogger returns a new AppLogger.
func NewAppLogger() *AppLogger {
	config := loadLogConfig()
	if config.format == logFormatJSON {
		appLogger := AppLogger{
			debugLogger: newJSONLogger(config),
			infoLogger:  newJSONLogger(config),
			debugLevel:  0,
		}
		return &appLogger
	}
	appLogger := AppLogger{
		debugLogger: newConsoleLogger(config),
		infoLogger:  newConsoleLogger(config),
		debugLevel:  0,
	}

	return &appLogger
}

// NewSilentLogger returns a new AppLogger that does not log anything.
func NewSilentLogger() *AppLogger {
	nop := zerolog.New(io.Discard)
	return &AppLogger{
		debugLogger: &nop,
		infoLogger:  &nop,
		debugLevel:  0,
	}
}

const (
	BoldWhite          = "\033[1;37m"
	HighIntensityBlack = "\033[90m"
	HighIntensityGreen = "\033[92m"
	BoldRed            = "\033[1;31m"
	Red                = "\033[31m"
	White              = "\033[97m"
	Reset              = "\033[0m"
)

var wrappedBuildErrorPattern = regexp.MustCompile(`^(.*exit status \d+) \(# [^\n]+\n([\s\S]*)\)$`)

// loadLogConfig returns the resolved logging configuration.
func loadLogConfig() logConfig {
	return logConfig{
		appEnv:     strings.TrimSpace(os.Getenv("APP_ENV")),
		appMode:    strings.TrimSpace(os.Getenv("APP_MODE")),
		format:     str.Of(os.Getenv(logFormatEnv)).TrimSpace().ToLower().String(),
		prefix:     strings.TrimSpace(os.Getenv("APP_LOG_PREFIX")),
		showCaller: os.Getenv("APP_LOG_CALLER") != "",
	}
}

// newConsoleLogger returns a console logger with the GoForj format.
func newConsoleLogger(config logConfig) *zerolog.Logger {
	prefix := logPrefix(config)
	output := zerolog.ConsoleWriter{Out: os.Stderr}
	output.FormatLevel = func(i interface{}) string {
		level, _ := i.(string)
		mark := ""
		switch level {
		case "warn":
			mark = fmt.Sprintf("%s!%s", HighIntensityBlack, Reset)
		case "error":
			mark = fmt.Sprintf("%s✖%s", Red, Reset)
		}
		if prefix == "" && mark == "" {
			return ""
		}
		if prefix == "" {
			return mark
		}
		if mark == "" {
			return fmt.Sprintf("%s ›", prefix)
		}
		return fmt.Sprintf("%s › %s", prefix, mark)
	}
	output.FormatMessage = func(i interface{}) string {
		return fmt.Sprintf("%s%s%s", White, i, Reset)
	}
	output.FormatFieldName = func(i interface{}) string {
		return fmt.Sprintf("%s· %s%s ", HighIntensityBlack, i, Reset)
	}
	output.FormatFieldValue = func(i interface{}) string {
		return fmt.Sprintf("%s%s%s", HighIntensityGreen, i, Reset)
	}
	output.FormatErrFieldName = func(i interface{}) string {
		return fmt.Sprintf("%s%s:%s ", HighIntensityBlack, i, Reset)
	}
	output.FormatErrFieldValue = func(i interface{}) string {
		return formatConsoleErrorValue(i)
	}
	output.FormatExtra = func(_ map[string]interface{}, buf *bytes.Buffer) error {
		if !config.showCaller {
			return nil
		}
		callerMeta := getCallerMeta()
		if callerMeta == "" {
			return nil
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(fmt.Sprintf("%s#%s%s", HighIntensityBlack, callerMeta, Reset))
		return nil
	}
	output.FormatTimestamp = func(i interface{}) string {
		return ""
	}

	logger := zerolog.New(output)
	return &logger
}

func formatConsoleErrorValue(i interface{}) string {
	raw := fmt.Sprintf("%s", i)
	if unquoted, err := strconv.Unquote(raw); err == nil {
		raw = unquoted
	}
	raw = strings.TrimSpace(raw)
	raw = normalizeWrappedBuildError(raw)
	if raw == "" {
		return fmt.Sprintf("%s%s", BoldRed, Reset)
	}
	lines := strings.Split(raw, "\n")
	if len(lines) == 1 {
		return fmt.Sprintf("%s%s%s", BoldRed, lines[0], Reset)
	}
	lines[0] = fmt.Sprintf("%s%s%s", BoldRed, lines[0], Reset)
	for idx := 1; idx < len(lines); idx++ {
		lines[idx] = fmt.Sprintf("%s  %s%s", BoldRed, strings.TrimLeft(lines[idx], " "), Reset)
	}
	return strings.Join(lines, "\n")
}

func normalizeWrappedBuildError(raw string) string {
	matches := wrappedBuildErrorPattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return raw
	}
	body := strings.TrimSpace(matches[2])
	if body == "" {
		return matches[1]
	}
	return matches[1] + "\n" + body
}

// newJSONLogger returns a JSON logger with optional prefix fields.
func newJSONLogger(config logConfig) *zerolog.Logger {
	base := zerolog.New(os.Stderr)
	ctx := base.With()
	app, component := splitPrefix(config.prefix)
	if app != "" {
		ctx = ctx.Str("app", app)
	}
	if component != "" {
		ctx = ctx.Str("component", component)
	}
	if config.appEnv != "" {
		ctx = ctx.Str("env", config.appEnv)
	}
	if config.appMode != "" {
		ctx = ctx.Str("app_mode", config.appMode)
	}
	if config.showCaller {
		ctx = ctx.Caller()
	}
	logger := ctx.Logger()
	return &logger
}

// getCallerMeta returns the caller type and package
// Example: QuestHotReloadWatcher (eqemuserver) ›
func getCallerMeta() string {
	pc := make([]uintptr, 20) // adjust the number of frames to retrieve
	n := runtime.Callers(0, pc)
	frames := runtime.CallersFrames(pc[:n])

	callerType := ""
	callerPackage := ""
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "/internal/logger.") || strings.Contains(frame.Function, "github.com/rs/zerolog") {
			continue
		}

		//fmt.Printf("- %s\n", frame.Function)
		if !more {
			break
		}

		pkg := frame.Function
		if strings.Contains(pkg, "(*") {
			callerType = pkg

			// extract type from github.com/Akkadius/spire/internal/eqemuserver.(*QuestHotReloadWatcher)
			// to QuestHotReloadWatcher
			split := strings.Split(pkg, "(*")
			if len(split) > 1 {
				callerType = str.Of(split[1]).ChopEnd(")").TrimSpace().ReplaceAll(")", "").String()

				// get package
				callerSplit := strings.Split(split[0], "/")
				if len(callerSplit) > 0 {
					callerPackage = callerSplit[len(callerSplit)-1]
					callerPackage = strings.ReplaceAll(callerPackage, ".", "")
				}
			}

			break
		}
	}

	var callerMeta string
	if callerType != "" {
		callerMeta = fmt.Sprintf("%s.%s", callerPackage, callerType)
	}

	return callerMeta
}

// logPrefix returns the formatted console prefix.
func logPrefix(config logConfig) string {
	raw := config.prefix
	if raw == "" {
		return ""
	}
	app, component := splitPrefix(raw)
	if config.appMode != "" {
		app = fmt.Sprintf("%s (%s)", app, config.appMode)
	}
	if component != "" {
		raw = fmt.Sprintf("%s › %s", app, component)
	} else {
		raw = app
	}
	return fmt.Sprintf("%s%s%s", BoldWhite, raw, Reset)
}

// splitPrefix splits "App › Component" into its parts.
func splitPrefix(value string) (string, string) {
	parts := strings.SplitN(value, "›", 2)
	if len(parts) == 0 {
		return "", ""
	}
	app := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return app, ""
	}
	component := strings.TrimSpace(parts[1])
	return app, component
}

// GetWriter returns the zerolog.Logger writer interface
func (l *AppLogger) GetWriter() zerolog.Logger {
	return l.infoLogger.With().Caller().Logger()
}

// Info is the default log event type
func (l *AppLogger) Info() *zerolog.Event {
	return l.infoLogger.Info()
}

// Error logs an error
func (l *AppLogger) Error() *zerolog.Event {
	return l.infoLogger.Error()
}

// Fatal logs a fatal error
func (l *AppLogger) Fatal() *zerolog.Event {
	return l.infoLogger.Fatal()
}

// Warn logs a warning
func (l *AppLogger) Warn() *zerolog.Event {
	return l.infoLogger.Warn()
}

// Debug is -v level logging
func (l *AppLogger) Debug() *zerolog.Event {
	if l.debugLevel >= 1 {
		return l.debugLogger.Debug()
	}
	return nil
}

// DebugVv is -vv level logging
func (l *AppLogger) DebugVv() *zerolog.Event {
	if l.debugLevel >= 2 {
		return l.debugLogger.Debug()
	}
	return nil
}

// DebugVvv is -vvv level logging
func (l *AppLogger) DebugVvv() *zerolog.Event {
	if l.debugLevel >= 3 {
		return l.debugLogger.Debug()
	}
	return nil
}

// SetDebugLevel sets the debug level (passed in from -v flags)
func (l *AppLogger) SetDebugLevel(level int) {
	l.debugLevel = level
}
