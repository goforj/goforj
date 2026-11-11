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

// inspectionFS keeps discovery opens and metadata within the caller-owned root.
type inspectionFS struct {
	root *os.Root
}

// InspectionFS provides confined read-only discovery without blocking on replacement FIFOs.
// The caller must keep root open for the lifetime of the returned filesystem.
func InspectionFS(root *os.Root) fs.FS {
	return inspectionFS{root: root}
}

// Open validates descriptor types before discovery can read a concurrently replaced path.
func (files inspectionFS) Open(name string) (fs.File, error) {
	local, err := inspectionPath("open", name)
	if err != nil {
		return nil, err
	}
	file, err := openInspectionFile(files.root, local)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		file.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("expected a regular file or directory")}
	}
	return file, nil
}

// Stat uses rooted metadata directly so discovery never opens marker contents to inspect their type.
func (files inspectionFS) Stat(name string) (fs.FileInfo, error) {
	local, err := inspectionPath("stat", name)
	if err != nil {
		return nil, err
	}
	return files.root.Stat(local)
}

// inspectionPath preserves io/fs path rules and rejects platform-specific separators before rooted access.
func inspectionPath(operation, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: operation, Path: name, Err: fs.ErrInvalid}
	}
	local, err := filepath.Localize(name)
	if err != nil {
		return "", &fs.PathError{Op: operation, Path: name, Err: fs.ErrInvalid}
	}
	return local, nil
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
	file, opened, err := openRegularFile(root, name, before)
	if err != nil {
		return nil, observedFile{}, err
	}
	defer file.Close()
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

// openRegularFile validates the opened descriptor because a checked path can be replaced before opening.
func openRegularFile(root *os.Root, name string, before fs.FileInfo) (*os.File, fs.FileInfo, error) {
	file, err := openInspectionFile(root, filepath.FromSlash(name))
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		file.Close()
		return nil, nil, fmt.Errorf("inspect %s: file changed while opening", name)
	}
	return file, opened, nil
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
	file, err := openInspectionFile(root, filepath.FromSlash(directory))
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	if !info.IsDir() {
		file.Close()
		return fmt.Errorf("inspect %s: expected a directory", directory)
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
