package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// createExclusivePrivateDirectory claims a backup set path without reusing existing contents.
func createExclusivePrivateDirectory(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	return nil
}

// ensurePrivateDirectory tightens a backup-owned directory even when it already exists.
func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set private directory permissions: %w", err)
	}
	return nil
}

// ensurePrivateRootDirectory tightens a backup-owned directory below an anchored filesystem root.
func ensurePrivateRootDirectory(root *os.Root, name string) error {
	if err := root.MkdirAll(name, 0o700); err != nil {
		return err
	}
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Chmod(0o700); err != nil {
		return fmt.Errorf("set private directory permissions: %w", err)
	}
	return nil
}

// openPrivateOutput opens an owner-only file and truncates it only after its permissions are enforced.
func openPrivateOutput(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := preparePrivateOutput(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

// openPrivateRootOutput applies owner-only permissions through an already anchored filesystem root.
func openPrivateRootOutput(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := preparePrivateOutput(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

// preparePrivateOutput prevents existing permissive files from exposing newly written backup data.
func preparePrivateOutput(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set private file permissions: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate private file: %w", err)
	}
	return nil
}

// writePrivateFile writes a complete owner-only backup file.
func writePrivateFile(path string, data []byte) error {
	file, err := openPrivateOutput(path)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

// writePrivateRootFile writes a complete owner-only file below an anchored filesystem root.
func writePrivateRootFile(root *os.Root, name string, data []byte) error {
	file, err := openPrivateRootOutput(root, name)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
