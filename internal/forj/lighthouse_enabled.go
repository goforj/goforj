package forj

import "github.com/goforj/env/v2"

// Enabled returns true if the lighthouse features are enabled.
func Enabled() bool {
	return env.GetBool("LIGHTHOUSE_ENABLED", "true")
}
