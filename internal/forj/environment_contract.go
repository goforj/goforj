package forj

import (
	"io/fs"

	"github.com/goforj/goforj/internal/envfile"
)

// WriteEnvironmentExampleAtomic redacts rendered environment contents before publishing the example through an atomic replacement.
func WriteEnvironmentExampleAtomic(path string, source []byte, defaultMode fs.FileMode) error {
	if err := envfile.ValidatePortableDocument(source); err != nil {
		return err
	}
	return writeFileAtomically(path, envfile.MergeExample(nil, source), defaultMode)
}
