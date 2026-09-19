package stacks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/env/v2"
)

// nearestRuntimeLayer follows the runtime loader's per-filename ancestor search without loading values into the process.
func nearestRuntimeLayer(root, name string) (file, error) {
	directory, err := filepath.Abs(root)
	if err != nil {
		return file{}, err
	}
	relative := name
	for level := 0; level < env.MaxDirectorySeekLevels; level++ {
		path := filepath.Join(directory, name)
		info, err := os.Stat(path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return file{}, fmt.Errorf("runtime layer %s must be a regular file", path)
			}
			data, err := os.ReadFile(path)
			return file{name: relative, before: data, exists: true}, err
		}
		if !os.IsNotExist(err) {
			return file{}, err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
		relative = filepath.Join("..", relative)
	}
	return file{}, nil
}
