package stacks

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// file retains absence and permissions for optimistic concurrency checks and rollback.
type file struct {
	name          string
	before, after []byte
	mode          fs.FileMode
	exists        bool
}

// isPrivateStackFile distinguishes working copies and recovery state from the public definition named local.
func isPrivateStackFile(name string) bool {
	profile, definition := strings.CutPrefix(name, ".env.stack.")
	return name == stateName || (definition && strings.HasSuffix(profile, ".local"))
}

// readFile rejects symlinks and special files before opening project-owned configuration.
func readFile(root, name string) (file, error) {
	f := file{name: name, mode: 0600}
	info, err := os.Lstat(filepath.Join(root, name))
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if !info.Mode().IsRegular() {
		return f, fmt.Errorf("%s must be a regular file", name)
	}
	f.exists = true
	f.mode = info.Mode().Perm()
	f.before, err = os.ReadFile(filepath.Join(root, name))
	return f, err
}

// checkFile prevents publication or rollback from replacing a destination changed by another writer.
func checkFile(root string, expected file) error {
	current, err := readFile(root, expected.name)
	if err != nil {
		return err
	}
	if current.exists != expected.exists || !bytes.Equal(current.before, expected.before) || current.mode != expected.mode {
		return fmt.Errorf("%s changed while the wizard was open; run forj stack again", expected.name)
	}
	return nil
}

// stage writes a complete replacement on the destination filesystem before publishing it.
func stage(root string, f file, data []byte) (string, error) {
	tmp, err := os.CreateTemp(root, ".env.stack-tmp-*.local")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(f.mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		if removeErr := os.Remove(name); removeErr != nil {
			return name, errors.Join(err, fmt.Errorf("remove temporary stack file: %w", removeErr))
		}
		return "", err
	}
	return name, nil
}

// commit checks all original files before writing and rolls back a failed replacement.
func commit(root string, files []file, rename func(string, string) error) error {
	// Publish ignore rules and recovery files before the active environment so an interrupted process retains the previous configuration.
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].name == ".gitignore" {
			return files[j].name != ".gitignore"
		}
		if files[j].name == ".gitignore" {
			return false
		}
		return files[i].name != ".env" && files[j].name == ".env"
	})
	for _, f := range files {
		if err := checkFile(root, f); err != nil {
			return err
		}
		if f.exists && f.mode&0222 == 0 {
			return fmt.Errorf("%s is read-only", f.name)
		}
	}
	for i, f := range files {
		replacement := f
		if isPrivateStackFile(f.name) {
			// Keep the original mode for conflict detection and rollback, but publish secrets only to their owner.
			replacement.mode = 0600
		}
		// Publish the ignore rules before creating any temporary file containing private settings.
		staged, err := stage(root, replacement, f.after)
		if err != nil {
			return rollback(root, files[:i], err, staged != "")
		}
		err = checkFile(root, f)
		privateConflict := err != nil && isPrivateStackFile(f.name)
		if err == nil {
			err = rename(staged, filepath.Join(root, f.name))
		}
		if err != nil {
			failure := fmt.Errorf("replace %s: %w", f.name, err)
			removeErr := os.Remove(staged)
			if removeErr != nil {
				failure = errors.Join(failure, fmt.Errorf("remove temporary stack file: %w", removeErr))
			}
			return rollback(root, files[:i], failure, removeErr != nil || privateConflict)
		}
	}
	return nil
}

// rollback restores published files in reverse order so ignore rules remain in place while private replacements are removed.
func rollback(root string, files []file, failure error, preserveIgnore bool) error {
	for j := len(files) - 1; j >= 0; j-- {
		original := files[j]
		if original.name == ".gitignore" && preserveIgnore {
			continue
		}
		published := original
		published.exists = true
		published.before = original.after
		if isPrivateStackFile(original.name) {
			published.mode = 0600
		}
		if err := checkFile(root, published); err != nil {
			preserveIgnore = true
			failure = errors.Join(failure, fmt.Errorf("restore %s: %w", original.name, err))
			continue
		}
		var restoreErr error
		if !original.exists {
			restoreErr = os.Remove(filepath.Join(root, original.name))
		} else {
			var backup string
			backup, restoreErr = stage(root, original, original.before)
			if restoreErr == nil {
				restoreErr = checkFile(root, published)
				if restoreErr == nil {
					restoreErr = os.Rename(backup, filepath.Join(root, original.name))
				}
				_ = os.Remove(backup)
			}
		}
		if restoreErr != nil {
			preserveIgnore = true
			failure = errors.Join(failure, fmt.Errorf("restore %s: %w", original.name, restoreErr))
		}
	}
	return failure
}
