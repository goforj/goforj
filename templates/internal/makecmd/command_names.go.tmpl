package makecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goforj/str/v2"
)

const nativeCommandNamesEnv = "FORJ_NATIVE_COMMAND_NAMES"

var commandMetadataPattern = regexp.MustCompile(`\b(name|aliases):"([^"]+)"`)

// CommandNameOwners supplies existing command ownership lazily so make:command can reject collisions after delegation.
type CommandNameOwners func() map[string]string

func (c *CommandCmd) validateCommandNameAvailable(name string) error {
	owners := map[string]string{}
	reserved := c.ReservedCommandNames
	if reserved == nil {
		reserved = delegatedNativeCommandNames
	}
	mergeCommandNameOwnerFunc(owners, reserved)
	mergeCommandNameOwners(owners, discoverProjectCommandNames("."))
	return validateCommandNameAvailable(name, owners)
}

func validateCommandNameAvailable(name string, owners map[string]string) error {
	clean := normalizeCommandName(name)
	if clean == "" {
		return nil
	}
	if owner, ok := owners[clean]; ok {
		return fmt.Errorf("command name %q is already used by %s; choose an app-specific name such as reports:sync", name, owner)
	}
	return nil
}

func mergeCommandNameOwners(dst, src map[string]string) {
	for name, owner := range src {
		clean := normalizeCommandName(name)
		if clean == "" {
			continue
		}
		dst[clean] = owner
	}
}

func mergeCommandNameOwnerFunc(dst map[string]string, src CommandNameOwners) {
	if src == nil {
		return
	}
	mergeCommandNameOwners(dst, src())
}

func delegatedNativeCommandNames() map[string]string {
	owners := map[string]string{}
	for _, name := range splitCommandNames(os.Getenv(nativeCommandNamesEnv)) {
		if clean := normalizeCommandName(name); clean != "" {
			owners[clean] = "GoForj command"
		}
	}
	return owners
}

func discoverProjectCommandNames(root string) map[string]string {
	owners := map[string]string{}
	for _, dir := range []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "migrations"),
	} {
		discoverCommandNamesInDir(dir, owners)
	}
	return owners
}

func discoverCommandNamesInDir(dir string, owners map[string]string) {
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, match := range commandMetadataPattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) != 3 {
				continue
			}
			for _, name := range splitCommandNames(match[2]) {
				if clean := normalizeCommandName(name); clean != "" {
					owners[clean] = fmt.Sprintf("generated App command metadata in %s", filepath.ToSlash(path))
				}
			}
		}
		return nil
	})
}

func splitCommandNames(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
}

func normalizeCommandName(name string) string {
	return str.Of(name).Trim().ToLower().String()
}
