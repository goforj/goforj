package scenarios

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	scenarioTreeEntryLimit = 25_000
	scenarioTreeByteLimit  = int64(2 << 30)
)

var (
	// ErrScenarioTreeEntryLimit identifies a scenario tree with too many paths to prepare safely.
	ErrScenarioTreeEntryLimit = errors.New("scenario tree entry limit exceeded")
	// ErrScenarioTreeByteLimit identifies a scenario tree whose regular file content exceeds the supported project limit.
	ErrScenarioTreeByteLimit = errors.New("scenario tree byte limit exceeded")
)

// copyScenarioTree preserves one immutable prepared base without following links outside that base.
func copyScenarioTree(sourceRoot, destinationRoot string) error {
	return copyScenarioTreeContext(context.Background(), sourceRoot, destinationRoot)
}

// copyScenarioTreeContext preserves one immutable prepared base without following links outside that base.
func copyScenarioTreeContext(ctx context.Context, sourceRoot, destinationRoot string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}
	type copiedDirectory struct {
		path string
		mode os.FileMode
	}
	var directories []copiedDirectory
	entries := 0
	bytes := int64(0)
	if err := filepath.WalkDir(sourceRoot, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		entries++
		if entries > scenarioTreeEntryLimit {
			return fmt.Errorf("%w: got more than %d paths", ErrScenarioTreeEntryLimit, scenarioTreeEntryLimit)
		}
		destinationPath := filepath.Join(destinationRoot, relative)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("prepared scenario contains unsupported symlink %q", relative)
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
		if err := checkScenarioTreeBytes(info.Size(), bytes); err != nil {
			return err
		}
		return copyScenarioFileContext(ctx, sourcePath, destinationPath, info.Mode().Perm(), &bytes)
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Chmod(directories[index].path, directories[index].mode); err != nil {
			return err
		}
	}
	return nil
}

// copyScenarioFile closes both sides explicitly so large fixtures do not exhaust descriptors during a walk.
func copyScenarioFile(source, destination string, mode os.FileMode) error {
	bytes := int64(0)
	return copyScenarioFileContext(context.Background(), source, destination, mode, &bytes)
}

// copyScenarioFileContext copies a file in cancellable chunks while applying the cumulative scenario-tree byte limit.
func copyScenarioFileContext(ctx context.Context, source, destination string, mode os.FileMode, bytes *int64) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	copyErr := copyScenarioContent(ctx, output, input, bytes)
	inputCloseErr := input.Close()
	outputCloseErr := output.Close()
	modeErr := os.Chmod(destination, mode)
	return errors.Join(copyErr, inputCloseErr, outputCloseErr, modeErr)
}

// copyScenarioContent copies content in bounded chunks so cancellation and the tree byte limit apply to individual large files.
func copyScenarioContent(ctx context.Context, destination io.Writer, source io.Reader, bytes *int64) error {
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			if int64(count) > scenarioTreeByteLimit-*bytes {
				return fmt.Errorf("%w: exceeds %d bytes", ErrScenarioTreeByteLimit, scenarioTreeByteLimit)
			}
			written, writeErr := destination.Write(buffer[:count])
			*bytes += int64(written)
			if writeErr != nil {
				return writeErr
			}
			if written != count {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// checkScenarioTreeBytes rejects known oversized files before copying or reading their content.
func checkScenarioTreeBytes(size, bytes int64) error {
	if size < 0 || size > scenarioTreeByteLimit-bytes {
		return fmt.Errorf("%w: exceeds %d bytes", ErrScenarioTreeByteLimit, scenarioTreeByteLimit)
	}
	return nil
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
	return digestScenarioTreeContext(context.Background(), root)
}

// digestScenarioTreeContext binds every prepared path, mode, symlink target, and regular file byte in lexical order.
func digestScenarioTreeContext(ctx context.Context, root string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if err != nil {
			return err
		}
		if path != root {
			if len(paths) == scenarioTreeEntryLimit {
				return fmt.Errorf("%w: got more than %d paths", ErrScenarioTreeEntryLimit, scenarioTreeEntryLimit)
			}
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	bytes := int64(0)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return "", err
		}
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
		if err := checkScenarioTreeBytes(info.Size(), bytes); err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		copyErr := copyScenarioContent(ctx, hash, file, &bytes)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
