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
	file, err := os.Create(artifact)
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
		input, err := os.Open(path)
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
	if _, err := safeArtifactPath(filepath.Dir(artifact), filepath.Base(artifact)); err != nil {
		return err
	}
	input, err := os.Open(artifact)
	if err != nil {
		return fmt.Errorf("open storage artifact: %w", err)
	}
	defer input.Close()
	zstdReader, err := zstd.NewReader(input)
	if err != nil {
		return fmt.Errorf("read storage artifact: %w", err)
	}
	defer zstdReader.Close()
	tarReader := tar.NewReader(zstdReader)
	root, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read storage archive: %w", err)
		}
		path, err := safeExtractPath(root, header.Name)
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if !header.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("unsupported storage archive entry %s", header.Name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode().Perm())
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
