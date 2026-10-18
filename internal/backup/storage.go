package backup

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// storageArchiveMaxEntries prevents archives from creating an unbounded number of filesystem objects.
const storageArchiveMaxEntries = 100_000

// storageArchiveMaxExpandedBytes leaves large application backups practical while bounding decompression work.
const storageArchiveMaxExpandedBytes int64 = 64 << 30

// archiveRestoreLimits bounds work accepted from a compressed storage artifact.
type archiveRestoreLimits struct {
	entries int
	bytes   int64
}

// ArchiveDirectory creates a zstd-compressed tar archive of a local storage directory.
func ArchiveDirectory(source string, artifact string) (resultErr error) {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat storage directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("storage source %s is not a directory", source)
	}
	if _, err := artifactPath(artifact); err != nil {
		return err
	}
	file, err := openPrivateOutput(artifact)
	if err != nil {
		return fmt.Errorf("create storage artifact: %w", err)
	}
	recordClose := func(operation string, closeFunc func() error) {
		if err := closeFunc(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("%s: %w", operation, err))
		}
	}
	defer recordClose("close storage artifact", file.Close)
	zstdWriter, err := zstd.NewWriter(file)
	if err != nil {
		return fmt.Errorf("create storage compressor: %w", err)
	}
	defer recordClose("close storage compressor", zstdWriter.Close)
	tarWriter := tar.NewWriter(zstdWriter)
	defer recordClose("close storage archive", tarWriter.Close)
	root := filepath.Clean(source)
	rootDirectory, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open storage source root: %w", err)
	}
	defer recordClose("close storage source root", rootDirectory.Close)
	err = filepath.Walk(root, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		header, err := tar.FileInfoHeader(entry, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !entry.Mode().IsRegular() {
			return nil
		}
		input, err := rootDirectory.Open(relative)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, input)
		closeErr := input.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("close storage source %s: %w", path, closeErr)
		}
		return errors.Join(copyErr, closeErr)
	})
	if err != nil {
		return fmt.Errorf("archive storage directory: %w", err)
	}
	return nil
}

// RestoreDirectoryArchive extracts a local storage archive into a directory.
func RestoreDirectoryArchive(artifact string, target string) error {
	return restoreDirectoryArchive(artifact, target, archiveRestoreLimits{
		entries: storageArchiveMaxEntries,
		bytes:   storageArchiveMaxExpandedBytes,
	})
}

// restoreDirectoryArchive extracts a local storage archive within explicit resource limits.
func restoreDirectoryArchive(artifact string, target string, limits archiveRestoreLimits) error {
	if _, err := safeArtifactPath(filepath.Dir(artifact), filepath.Base(artifact)); err != nil {
		return err
	}
	input, err := os.Open(artifact)
	if err != nil {
		return fmt.Errorf("open storage artifact: %w", err)
	}
	defer input.Close()
	root, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := validateDirectoryArchive(input, root, limits); err != nil {
		return err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind storage artifact: %w", err)
	}
	zstdReader, err := zstd.NewReader(input)
	if err != nil {
		return fmt.Errorf("read storage artifact: %w", err)
	}
	defer zstdReader.Close()
	tarReader := tar.NewReader(zstdReader)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	targetDirectory, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open storage restore root: %w", err)
	}
	defer targetDirectory.Close()
	entries := 0
	var expandedBytes int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read storage archive: %w", err)
		}
		relative, err := checkedArchiveEntry(root, header, limits, &entries, &expandedBytes)
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			if err := targetDirectory.MkdirAll(relative, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := targetDirectory.MkdirAll(filepath.Dir(relative), 0o755); err != nil {
			return err
		}
		output, err := targetDirectory.OpenFile(relative, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, tarReader)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// validateDirectoryArchive rejects an invalid archive before the live target can be mutated.
func validateDirectoryArchive(input io.Reader, root string, limits archiveRestoreLimits) error {
	zstdReader, err := zstd.NewReader(input)
	if err != nil {
		return fmt.Errorf("read storage artifact: %w", err)
	}
	defer zstdReader.Close()
	tarReader := tar.NewReader(zstdReader)
	entries := 0
	var expandedBytes int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read storage archive: %w", err)
		}
		if _, err := checkedArchiveEntry(root, header, limits, &entries, &expandedBytes); err != nil {
			return err
		}
	}
}

// checkedArchiveEntry validates one entry and advances the restore resource counters.
func checkedArchiveEntry(root string, header *tar.Header, limits archiveRestoreLimits, entries *int, expandedBytes *int64) (string, error) {
	*entries = *entries + 1
	if *entries > limits.entries {
		return "", fmt.Errorf("storage archive exceeds %d entries", limits.entries)
	}
	if header.Size < 0 || header.Size > limits.bytes-*expandedBytes {
		return "", fmt.Errorf("storage archive exceeds %d expanded bytes", limits.bytes)
	}
	*expandedBytes += header.Size
	path, err := safeExtractPath(root, header.Name)
	if err != nil {
		return "", err
	}
	if !header.FileInfo().IsDir() && !header.FileInfo().Mode().IsRegular() {
		return "", fmt.Errorf("unsupported storage archive entry %s", header.Name)
	}
	return filepath.Rel(root, path)
}

// safeExtractPath prevents archive entries from escaping the target directory.
func safeExtractPath(root string, name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid storage archive path %q", name)
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage archive path escapes target: %s", name)
	}
	return path, nil
}
