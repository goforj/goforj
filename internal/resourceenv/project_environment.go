package resourceenv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// ReadProjectEnvironment reads the project-owned dotenv layers without changing process state.
func ReadProjectEnvironment(root string) (map[string]string, error) {
	values := map[string]string{}
	for _, name := range []string{".env.example", ".env"} {
		path := filepath.Join(root, name)
		loaded, err := godotenv.Read(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		for key, value := range loaded {
			values[key] = value
		}
	}
	return values, nil
}
