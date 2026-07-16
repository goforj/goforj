package forj

import (
	"io/fs"

	"github.com/goforj/goforj/internal/envfile"
)

// WriteEnvironmentExampleAtomic redacts rendered environment contents before publishing the example through an atomic replacement.
func WriteEnvironmentExampleAtomic(path string, source []byte, defaultMode fs.FileMode) error {
	return writeFileAtomically(path, envfile.RedactExample(source), defaultMode)
}
