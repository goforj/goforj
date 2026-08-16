// Package appassets records lightweight filesystem snapshots for App-owned frontend builds.
package appassets

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const snapshotVersion = 2

// Asset identifies one App-owned frontend and the command that produces its dist output.
type Asset struct {
	App     string
	Name    string
	Root    string
	Prepare string
	Command string
}

// treeState summarizes a canonical metadata walk without retaining one receipt record per file.
type treeState struct {
	Files  int    `json:"files"`
	Digest string `json:"digest"`
}

// snapshot is published only after an asset command and its output validation succeed.
type snapshot struct {
	Version int       `json:"version"`
	Prepare string    `json:"prepare,omitempty"`
	Command string    `json:"command"`
	Inputs  treeState `json:"inputs"`
	Outputs treeState `json:"outputs"`
}

// Current reports whether the current source and dist metadata match the last successful build.
func Current(projectRoot string, asset Asset) (bool, error) {
	paths, err := resolvePaths(projectRoot, asset)
	if err != nil {
		return false, err
	}
	recorded, err := readSnapshot(paths.receipt)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errInvalidSnapshot) {
			return false, nil
		}
		return false, err
	}
	if recorded.Version != snapshotVersion ||
		recorded.Prepare != strings.TrimSpace(asset.Prepare) ||
		recorded.Command != strings.TrimSpace(asset.Command) {
		return false, nil
	}
	inputs, err := scanInputs(paths.assetRoot)
	if err != nil {
		return false, err
	}
	outputs, err := scanOutputs(paths.outputRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return recorded.Inputs == inputs && recorded.Outputs == outputs, nil
}

// Record publishes the current metadata only after the caller successfully builds the asset.
func Record(projectRoot string, asset Asset) error {
	paths, err := resolvePaths(projectRoot, asset)
	if err != nil {
		return err
	}
	inputs, err := scanInputs(paths.assetRoot)
	if err != nil {
		return err
	}
	outputs, err := scanOutputs(paths.outputRoot)
	if err != nil {
		return fmt.Errorf("inspect asset output: %w", err)
	}
	if outputs.Files == 0 {
		return fmt.Errorf("inspect asset output %q: dist contains no files", paths.outputRoot)
	}
	return writeSnapshot(paths.receipt, snapshot{
		Version: snapshotVersion,
		Prepare: strings.TrimSpace(asset.Prepare),
		Command: strings.TrimSpace(asset.Command),
		Inputs:  inputs,
		Outputs: outputs,
	})
}

type resolvedPaths struct {
	assetRoot  string
	outputRoot string
	receipt    string
}

// resolvePaths keeps configured frontend and receipt paths within their owning Project.
func resolvePaths(projectRoot string, asset Asset) (resolvedPaths, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return resolvedPaths{}, fmt.Errorf("resolve project root: %w", err)
	}
	if !safeName(asset.App) || !safeName(asset.Name) {
		return resolvedPaths{}, fmt.Errorf("asset identity %q/%q must use lowercase letters, digits, and hyphens", asset.App, asset.Name)
	}
	assetRoot := strings.TrimSpace(asset.Root)
	if assetRoot == "" {
		return resolvedPaths{}, fmt.Errorf("asset %s/%s path is required", asset.App, asset.Name)
	}
	if !filepath.IsAbs(assetRoot) {
		assetRoot = filepath.Join(projectRoot, assetRoot)
	}
	assetRoot, err = filepath.Abs(assetRoot)
	if err != nil {
		return resolvedPaths{}, fmt.Errorf("resolve asset %s/%s path: %w", asset.App, asset.Name, err)
	}
	relative, err := filepath.Rel(projectRoot, assetRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return resolvedPaths{}, fmt.Errorf("asset %s/%s path must stay within the project root", asset.App, asset.Name)
	}
	return resolvedPaths{
		assetRoot:  assetRoot,
		outputRoot: filepath.Join(assetRoot, "dist"),
		receipt:    filepath.Join(projectRoot, "bin", ".forj-build-cache", "spas", asset.App, asset.Name+".json"),
	}, nil
}

var errInvalidSnapshot = errors.New("invalid asset snapshot")

// readSnapshot treats malformed local tool state as a cache miss rather than trusting partial data.
func readSnapshot(path string) (snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot{}, err
	}
	var value snapshot
	if err := json.Unmarshal(data, &value); err != nil {
		return snapshot{}, errInvalidSnapshot
	}
	return value, nil
}

// writeSnapshot atomically advances the receipt so interrupted writes cannot create a false cache hit.
func writeSnapshot(path string, value snapshot) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode asset snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create asset snapshot directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
	if err != nil {
		return fmt.Errorf("create asset snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write asset snapshot: %w", err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect asset snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close asset snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish asset snapshot: %w", err)
	}
	return nil
}

// scanInputs inventories source metadata while pruning dependency and output trees before descent.
func scanInputs(root string) (treeState, error) {
	return scanTree(root, func(relative string, entry os.DirEntry) bool {
		if !entry.IsDir() {
			return false
		}
		name := entry.Name()
		return name == "node_modules" || relative == "dist"
	})
}

// scanOutputs inventories emitted files separately so missing or externally changed dist output invalidates the receipt.
func scanOutputs(root string) (treeState, error) {
	return scanTree(root, nil)
}

// scanTree streams deterministic metadata without opening regular file contents or retaining per-file records.
func scanTree(root string, skip func(relative string, entry os.DirEntry) bool) (treeState, error) {
	encoded := make([]byte, 0, 4096)
	files := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if skip != nil && skip(relative, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			linkTarget = target
		}
		encoded = appendTreeStateString(encoded, relative)
		encoded = binary.LittleEndian.AppendUint64(encoded, uint64(info.Size()))
		encoded = binary.LittleEndian.AppendUint64(encoded, uint64(info.ModTime().UnixNano()))
		encoded = binary.LittleEndian.AppendUint32(encoded, uint32(info.Mode()))
		encoded = appendTreeStateString(encoded, linkTarget)
		files++
		return nil
	})
	if err != nil {
		return treeState{}, err
	}
	digest := sha256.Sum256(encoded)
	return treeState{Files: files, Digest: hex.EncodeToString(digest[:])}, nil
}

// appendTreeStateString length-prefixes variable data so distinct paths and link targets cannot share one byte stream.
func appendTreeStateString(encoded []byte, value string) []byte {
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(len(value)))
	return append(encoded, value...)
}

// safeName keeps cache paths independent from untrusted App and asset names.
func safeName(value string) bool {
	if value == "" || value == "." || value == ".." || strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' || index > 0 && character == '-' {
			continue
		}
		return false
	}
	return true
}
