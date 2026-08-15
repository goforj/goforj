package scenarios

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// copyScenarioTree preserves one immutable prepared base without following links outside that base.
func copyScenarioTree(sourceRoot, destinationRoot string) error {
	sourceRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}
	type copiedDirectory struct {
		path string
		mode os.FileMode
	}
	var directories []copiedDirectory
	if err := filepath.WalkDir(sourceRoot, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destinationPath := filepath.Join(destinationRoot, relative)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(sourcePath)
			if err != nil {
				return err
			}
			if filepath.IsAbs(target) {
				return fmt.Errorf("prepared scenario contains absolute symlink %q", relative)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), target))
			relativeTarget, err := filepath.Rel(sourceRoot, resolved)
			if err != nil || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) {
				return fmt.Errorf("prepared scenario symlink %q escapes its root", relative)
			}
			return os.Symlink(target, destinationPath)
		}
		if entry.IsDir() {
			if err := os.MkdirAll(destinationPath, 0o700); err != nil {
				return err
			}
			directories = append(directories, copiedDirectory{path: destinationPath, mode: info.Mode().Perm()})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared scenario contains unsupported path %q", relative)
		}
		return copyScenarioFile(sourcePath, destinationPath, info.Mode().Perm())
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := os.Chmod(directories[index].path, directories[index].mode); err != nil {
			return err
		}
	}
	return nil
}

// copyScenarioFile closes both sides explicitly so large fixtures do not exhaust descriptors during a walk.
func copyScenarioFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, input.Close(), output.Close())
}

// removeScenarioTree restores directory traversal only within the owned root so read-only fixtures remain disposable.
func removeScenarioTree(root string) error {
	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	})
	return errors.Join(walkErr, os.RemoveAll(root))
}

// digestScenarioTree binds every prepared path, mode, symlink target, and regular file byte in lexical order.
func digestScenarioTree(root string) (string, error) {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", filepath.ToSlash(relative), info.Mode().String())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(hash, "%s\x00", target)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
