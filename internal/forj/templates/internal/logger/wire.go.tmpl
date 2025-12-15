package logger

import (
	"github.com/goforj/env"
	"os"
	"strings"
)

// ProvideAppLogger is a function that provides an instance of AppLogger.
func ProvideAppLogger() *AppLogger {
	l := NewAppLogger()

	// If the user supplies -v, -vv, -vvv, etc, set the logger verbosity.
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-v") {
			l.SetDebugLevel(strings.Count(arg, "v"))
		}
	}

	// If the user supplies APP_DEBUG=1-3 to set the logger verbosity.
	if len(os.Getenv("APP_DEBUG")) > 0 {
		l.SetDebugLevel(env.GetInt("APP_DEBUG", "0"))
	}

	return l
}
