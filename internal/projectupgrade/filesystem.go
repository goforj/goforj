package projectupgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// observedFile retains the exact compared state for a second check before a plan can be returned.
type observedFile struct {
	exists        bool
	digest        string
	mode          uint32
	captureSource bool
}

// ReadInput reads one bounded, stable project input through the same confined boundary as managed source inspection.
func ReadInput(directory, name string) ([]byte, error) {
	if !validArtifactPath(name) {
		return nil, fmt.Errorf("invalid project input path")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, state, err := readRegular(root, name)
	if err != nil {
		return nil, err
	}
	if !state.exists {
		return nil, fmt.Errorf("read %s: %w", name, fs.ErrNotExist)
	}
	return data, nil
}

// checkedFileInfo rejects symlinks using a rooted handle so concurrent path replacement cannot escape the application.
func checkedFileInfo(root *os.Root, name string) (fs.FileInfo, error) {
	parts := strings.Split(name, "/")
	for index := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:index+1], "/")))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("inspect %s: symlinks are not supported", name)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("inspect %s: parent is not a directory", name)
		}
		if index == len(parts)-1 {
			return info, nil
		}
	}
	return nil, fmt.Errorf("empty artifact path")
}

// readRegular bounds the opened file and verifies its identity before and after reading instead of trusting a prior path check.
func readRegular(root *os.Root, name string) ([]byte, observedFile, error) {
	return readManaged(root, name, true)
}

// readManaged streams raw-asset fingerprints while bounding source payloads and detecting concurrent replacement.
func readManaged(root *os.Root, name string, captureSource bool) ([]byte, observedFile, error) {
	limit := int64(4 << 20)
	if !captureSource {
		limit = 64 << 20
	}
	before, err := checkedFileInfo(root, name)
	if err != nil || before == nil {
		return nil, observedFile{captureSource: captureSource}, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, observedFile{}, fmt.Errorf("inspect %s: expected a regular file of at most %d MiB", name, limit>>20)
	}
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, observedFile{}, fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, observedFile{}, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return nil, observedFile{}, fmt.Errorf("inspect %s: file changed while opening", name)
	}
	var data []byte
	var size int64
	var digest string
	if captureSource {
		data, err = io.ReadAll(io.LimitReader(file, limit+1))
		size = int64(len(data))
		digest = Digest(data)
	} else {
		hash := sha256.New()
		size, err = io.Copy(hash, io.LimitReader(file, limit+1))
		digest = hex.EncodeToString(hash.Sum(nil))
	}
	if err != nil {
		return nil, observedFile{}, fmt.Errorf("read %s: %w", name, err)
	}
	after, err := file.Stat()
	if err != nil {
		return nil, observedFile{}, err
	}
	current, err := checkedFileInfo(root, name)
	if err != nil {
		return nil, observedFile{}, err
	}
	if size > limit || size != opened.Size() || current == nil || !os.SameFile(opened, current) ||
		!opened.ModTime().Equal(after.ModTime()) || after.Size() != opened.Size() || !opened.ModTime().Equal(current.ModTime()) ||
		opened.Mode() != after.Mode() || opened.Mode() != current.Mode() {
		return nil, observedFile{}, fmt.Errorf("inspect %s: file changed while reading", name)
	}
	return data, observedFile{exists: true, digest: digest, mode: uint32(opened.Mode().Perm()), captureSource: captureSource}, nil
}

// verifyObservations refuses a mixed or stale plan when any compared managed file changes during inspection.
func verifyObservations(root *os.Root, observations map[string]observedFile) error {
	for name, expected := range observations {
		_, current, err := readManaged(root, name, expected.captureSource)
		if err != nil {
			return err
		}
		if current != expected {
			return fmt.Errorf("inspect %s: file changed during planning; retry the inspection", name)
		}
	}
	return nil
}

// preserveUnknown inventories additions through the rooted handle without opening owner files or following directory symlinks.
func preserveUnknown(root *os.Root, directory string, known, prefixes map[string]bool, plan *Plan) error {
	if directory == "" {
		directory = "."
	}
	file, err := root.Open(filepath.FromSlash(directory))
	if err != nil {
		return err
	}
	entries, err := file.ReadDir(-1)
	file.Close()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := path.Join(directory, entry.Name())
		if entry.Name() == ".git" || known[name] {
			continue
		}
		if privateArtifact(name) {
			plan.Files = append(plan.Files, File{Path: name, Ownership: "private", Action: "preserve-private"})
			continue
		}
		if entry.IsDir() && prefixes[name] {
			if _, err := checkedFileInfo(root, name); err != nil {
				return err
			}
			if err := preserveUnknown(root, name, known, prefixes, plan); err != nil {
				return err
			}
			continue
		}
		plan.Files = append(plan.Files, File{Path: name, Ownership: "unknown-neighbor", Action: "preserve"})
	}
	return nil
}
