package backup

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// Checksum returns a SHA-256 checksum in the manifest format.
func Checksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open backup artifact: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, fmt.Errorf("hash backup artifact: %w", err)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), size, nil
}

// VerifyChecksum verifies one artifact against a SHA-256 manifest checksum.
func VerifyChecksum(path string, expected string) error {
	actual, _, err := Checksum(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("backup checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
