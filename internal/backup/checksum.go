package backup

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// ArtifactFingerprint records the integrity metadata derived while reading one artifact.
type ArtifactFingerprint struct {
	Checksum string
	Size     int64
}

// Checksum returns the integrity metadata needed to describe an artifact in a manifest.
func Checksum(path string) (ArtifactFingerprint, error) {
	file, err := os.Open(path)
	if err != nil {
		return ArtifactFingerprint{}, fmt.Errorf("open backup artifact: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return ArtifactFingerprint{}, fmt.Errorf("hash backup artifact: %w", err)
	}
	return ArtifactFingerprint{Checksum: fmt.Sprintf("sha256:%x", hash.Sum(nil)), Size: size}, nil
}

// VerifyChecksum verifies one artifact against a SHA-256 manifest checksum.
func VerifyChecksum(path string, expected string) error {
	fingerprint, err := Checksum(path)
	if err != nil {
		return err
	}
	if fingerprint.Checksum != expected {
		return fmt.Errorf("backup checksum mismatch: expected %s, got %s", expected, fingerprint.Checksum)
	}
	return nil
}
