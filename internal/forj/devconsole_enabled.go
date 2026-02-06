package forj

import "github.com/goforj/env/v2"

// Enabled returns true if the devconsole features are enabled.
func Enabled() bool {
	return env.GetBool("DEVCONSOLE_ENABLED", "true")
}
