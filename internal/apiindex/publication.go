package apiindex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fileSnapshot distinguishes a missing artifact from a present empty artifact.
type fileSnapshot struct {
	content []byte
	exists  bool
}

// preparedCandidate keeps validated candidates isolated until the final build step proves they are safe to expose.
type preparedCandidate struct {
	paths                 paths
	stagedPaths           paths
	stagingDir            string
	active                snapshots
	candidates            snapshots
	report                runReport
	remove                bool
	locks                 artifactLockCoordinator
	renameFile            func(string, string) error
	removeStagingDir      func(string) error
	afterArtifactMutation func(int, string)
}

// publication pairs one candidate with the active snapshot needed for safe rollback.
type publication struct {
	activePath   string
	stagedPath   string
	active       fileSnapshot
	candidate    fileSnapshot
	wasPublished bool
}

// removal retains a stale artifact under a hidden sibling name until cleanup can commit as a set.
type removal struct {
	activePath    string
	tombstonePath string
	active        fileSnapshot
	wasMoved      bool
}

// snapshots represents one complete generation of API artifacts.
type snapshots struct {
	out         fileSnapshot
	diagnostics fileSnapshot
	openAPI     fileSnapshot
}

// readSnapshots reads every active or staged artifact before making a lifecycle decision.
func readSnapshots(paths paths) (snapshots, error) {
	out, err := readFileSnapshot(paths.out)
	if err != nil {
		return snapshots{}, err
	}
	diagnostics, err := readFileSnapshot(paths.diagnostics)
	if err != nil {
		return snapshots{}, err
	}
	openAPI, err := readFileSnapshot(paths.openAPI)
	if err != nil {
		return snapshots{}, err
	}
	return snapshots{
		out:         out,
		diagnostics: diagnostics,
		openAPI:     openAPI,
	}, nil
}

// readValidatedSnapshots rejects missing or malformed candidates before any active artifact can be replaced.
func readValidatedSnapshots(paths paths) (snapshots, error) {
	candidateSnapshots, err := readSnapshots(paths)
	if err != nil {
		return snapshots{}, err
	}
	for _, artifact := range []struct {
		name     string
		path     string
		snapshot fileSnapshot
	}{
		{name: "API index", path: paths.out, snapshot: candidateSnapshots.out},
		{name: "API index diagnostics", path: paths.diagnostics, snapshot: candidateSnapshots.diagnostics},
		{name: "OpenAPI", path: paths.openAPI, snapshot: candidateSnapshots.openAPI},
	} {
		if !artifact.snapshot.exists {
			return snapshots{}, fmt.Errorf("validate staged %s artifact %q: file was not generated", artifact.name, artifact.path)
		}
		if !json.Valid(artifact.snapshot.content) {
			return snapshots{}, fmt.Errorf("validate staged %s artifact %q: invalid JSON", artifact.name, artifact.path)
		}
	}
	return candidateSnapshots, nil
}

// createStagingDir locates candidates beside their final paths so each publication is an atomic rename.
func createStagingDir(paths paths) (string, error) {
	parent, err := artifactDirectory(paths)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create API index artifact directory %q: %w", parent, err)
	}
	stagingDir, err := os.MkdirTemp(parent, ".forj-api-index-stage-")
	if err != nil {
		return "", fmt.Errorf("create API index staging directory in %q: %w", parent, err)
	}
	return stagingDir, nil
}

// stagedPaths redirects only generated artifacts while retaining the real source and composition roots.
func stagedPaths(paths paths, stagingDir string) paths {
	staged := paths
	staged.out = filepath.Join(stagingDir, filepath.Base(paths.out))
	staged.diagnostics = filepath.Join(stagingDir, filepath.Base(paths.diagnostics))
	staged.openAPI = filepath.Join(stagingDir, filepath.Base(paths.openAPI))
	return staged
}

// seedStagedArtifacts lets Web's changed-only publisher reuse active bytes without rewriting and syncing a complete unchanged generation.
func seedStagedArtifacts(activePaths paths, candidatePaths paths, active snapshots) error {
	artifacts := []struct {
		activePath    string
		candidatePath string
		snapshot      fileSnapshot
	}{
		{activePath: activePaths.out, candidatePath: candidatePaths.out, snapshot: active.out},
		{activePath: activePaths.diagnostics, candidatePath: candidatePaths.diagnostics, snapshot: active.diagnostics},
		{activePath: activePaths.openAPI, candidatePath: candidatePaths.openAPI, snapshot: active.openAPI},
	}
	for _, artifact := range artifacts {
		if !artifact.snapshot.exists {
			continue
		}
		if err := seedStagedArtifact(artifact.activePath, artifact.candidatePath, artifact.snapshot); err != nil {
			return err
		}
	}
	return nil
}

// seedStagedArtifact falls back to a private copy when the active and staging directories cannot share hard links.
func seedStagedArtifact(activePath string, candidatePath string, snapshot fileSnapshot) error {
	var linkErr error
	activeInfo, err := os.Lstat(activePath)
	if err == nil && activeInfo.Mode().IsRegular() {
		linkErr = os.Link(activePath, candidatePath)
		if linkErr == nil {
			candidateInfo, err := os.Lstat(candidatePath)
			if err == nil && candidateInfo.Mode().IsRegular() {
				return nil
			}
			linkErr = err
			if linkErr == nil {
				linkErr = fmt.Errorf("hard-linked staged artifact is not a regular file")
			}
			if err := os.Remove(candidatePath); err != nil {
				return fmt.Errorf("remove unsafe staged API index link %q: %w", candidatePath, err)
			}
		}
	} else if err != nil {
		linkErr = err
	}
	file, err := os.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if linkErr != nil {
			err = errors.Join(linkErr, err)
		}
		return fmt.Errorf("seed staged API index artifact %q from %q: %w", candidatePath, activePath, err)
	}
	removeCandidate := true
	defer func() {
		if removeCandidate {
			_ = os.Remove(candidatePath)
		}
	}()
	if _, err := file.Write(snapshot.content); err != nil {
		_ = file.Close()
		return fmt.Errorf("copy active API index artifact %q to staging: %w", activePath, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync staged API index artifact %q: %w", candidatePath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staged API index artifact %q: %w", candidatePath, err)
	}
	removeCandidate = false
	return nil
}

// discardIfActiveUnchanged removes an unchanged candidate while holding the publication lock that protects its active generation.
func (p *preparedCandidate) discardIfActiveUnchanged() (unchanged bool, err error) {
	lock, err := p.locks.acquire(p.paths)
	if err != nil {
		return false, err
	}
	defer func() {
		err = errors.Join(err, lock.Release())
	}()
	linked, err := artifactGenerationsShareFiles(p.paths, p.stagedPaths)
	if err != nil {
		return false, err
	}
	if !linked {
		active, err := readSnapshots(p.paths)
		if err != nil {
			return false, fmt.Errorf("confirm unchanged API index artifacts: %w", err)
		}
		if !active.equal(p.active) {
			return false, nil
		}
	}
	if err := p.Discard(); err != nil {
		return false, err
	}
	return true, nil
}

// artifactGenerationsShareFiles confirms that every staged artifact still names the same inode as its active counterpart.
func artifactGenerationsShareFiles(activePaths paths, candidatePaths paths) (bool, error) {
	artifacts := [][2]string{
		{activePaths.out, candidatePaths.out},
		{activePaths.diagnostics, candidatePaths.diagnostics},
		{activePaths.openAPI, candidatePaths.openAPI},
	}
	for _, artifact := range artifacts {
		activeInfo, err := os.Stat(artifact[0])
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("inspect active API index artifact %q: %w", artifact[0], err)
		}
		candidateInfo, err := os.Stat(artifact[1])
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("inspect staged API index artifact %q: %w", artifact[1], err)
		}
		if !os.SameFile(activeInfo, candidateInfo) {
			return false, nil
		}
	}
	return true, nil
}

// Publish atomically promotes this candidate and preserves App context on publication failures.
func (p *preparedCandidate) Publish() error {
	if err := p.publish(); err != nil {
		report := p.report
		if strings.TrimSpace(report.appName) == "" {
			report.appName = p.paths.appName
		}
		report.outcome = outcomeRejected
		return fmt.Errorf("%s: %w", report.status(), err)
	}
	return nil
}

// publish serializes the complete compare-and-swap transaction with direct webindex publishers.
func (p *preparedCandidate) publish() (err error) {
	if !p.remove && p.stagingDir == "" {
		return fmt.Errorf("publish API index: candidate has no staging directory")
	}
	lock, err := p.locks.acquire(p.paths)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, lock.Release())
	}()
	return p.publishLocked()
}

// publishLocked exposes a validated candidate set or applies deferred CLI-only cleanup while holding the shared lock.
func (p *preparedCandidate) publishLocked() error {
	active, err := readSnapshots(p.paths)
	if err != nil {
		return fmt.Errorf("publish API index: read active artifacts: %w", err)
	}
	if !active.equal(p.active) {
		if p.remove && !active.anyExists() {
			return nil
		}
		if !p.remove && active.equal(p.candidates) {
			return nil
		}
		return fmt.Errorf("publish API index: active artifacts changed after candidate preparation")
	}
	if p.remove {
		return p.removeArtifacts(active)
	}
	staged, err := readValidatedSnapshots(p.stagedPaths)
	if err != nil {
		return err
	}
	if !staged.equal(p.candidates) {
		return fmt.Errorf("publish API index: staged artifacts changed after validation")
	}
	artifacts := []publication{
		{activePath: p.paths.out, stagedPath: p.stagedPaths.out, active: active.out, candidate: staged.out},
		{activePath: p.paths.diagnostics, stagedPath: p.stagedPaths.diagnostics, active: active.diagnostics, candidate: staged.diagnostics},
		{activePath: p.paths.openAPI, stagedPath: p.stagedPaths.openAPI, active: active.openAPI, candidate: staged.openAPI},
	}
	for index := range artifacts {
		artifact := &artifacts[index]
		if artifact.active.equal(artifact.candidate) {
			continue
		}
		latest, err := readFileSnapshot(artifact.activePath)
		if err != nil {
			return rollbackPublicationError(artifacts[:index], fmt.Errorf("recheck active API index artifact %q: %w", artifact.activePath, err))
		}
		if !latest.equal(artifact.active) {
			return rollbackPublicationError(artifacts[:index], fmt.Errorf("active API index artifact %q changed during publication", artifact.activePath))
		}
		if err := p.renameArtifact(artifact.stagedPath, artifact.activePath); err != nil {
			return rollbackPublicationError(artifacts[:index], fmt.Errorf("publish API index artifact %q: %w", artifact.activePath, err))
		}
		artifact.wasPublished = true
		p.noteArtifactMutation(index, artifact.activePath)
	}
	return nil
}

// removeArtifacts renames every stale artifact aside before committing their deletion as one coordinated set.
func (p *preparedCandidate) removeArtifacts(active snapshots) error {
	removals := []removal{
		{activePath: p.paths.out, active: active.out},
		{activePath: p.paths.diagnostics, active: active.diagnostics},
		{activePath: p.paths.openAPI, active: active.openAPI},
	}
	for index := range removals {
		removal := &removals[index]
		if !removal.active.exists {
			continue
		}
		latest, err := readFileSnapshot(removal.activePath)
		if err != nil {
			return p.rollbackRemovalError(removals[:index], fmt.Errorf("recheck stale API index artifact %q: %w", removal.activePath, err))
		}
		if !latest.equal(removal.active) {
			return p.rollbackRemovalError(removals[:index], fmt.Errorf("stale API index artifact %q changed during cleanup", removal.activePath))
		}
		removal.tombstonePath, err = reserveTombstone(removal.activePath)
		if err != nil {
			return p.rollbackRemovalError(removals[:index], err)
		}
		if err := p.renameArtifact(removal.activePath, removal.tombstonePath); err != nil {
			return p.rollbackRemovalError(removals[:index], fmt.Errorf("stage stale API index artifact %q for removal: %w", removal.activePath, err))
		}
		removal.wasMoved = true
		p.noteArtifactMutation(index, removal.activePath)
	}

	var disposalErr error
	for _, removal := range removals {
		if !removal.wasMoved {
			continue
		}
		if err := os.Remove(removal.tombstonePath); err != nil && !os.IsNotExist(err) {
			disposalErr = errors.Join(disposalErr, fmt.Errorf("remove stale API index tombstone %q: %w", removal.tombstonePath, err))
		}
	}
	return disposalErr
}

// reserveTombstone allocates a collision-resistant sibling name without crossing filesystems.
func reserveTombstone(path string) (string, error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".forj-api-index-remove-")
	if err != nil {
		return "", fmt.Errorf("reserve API index tombstone beside %q: %w", path, err)
	}
	tombstonePath := temporary.Name()
	closeErr := temporary.Close()
	removeErr := os.Remove(tombstonePath)
	if closeErr != nil || removeErr != nil {
		var reservationErr error
		if closeErr != nil {
			reservationErr = errors.Join(reservationErr, fmt.Errorf("close API index tombstone reservation %q: %w", tombstonePath, closeErr))
		}
		if removeErr != nil {
			reservationErr = errors.Join(reservationErr, fmt.Errorf("clear API index tombstone reservation %q: %w", tombstonePath, removeErr))
		}
		return "", reservationErr
	}
	return tombstonePath, nil
}

// rollbackRemovalError preserves the cleanup failure and every failed tombstone restoration.
func (p *preparedCandidate) rollbackRemovalError(moved []removal, cleanupErr error) error {
	rollbackErr := p.rollbackRemovals(moved)
	if rollbackErr == nil {
		return cleanupErr
	}
	return errors.Join(cleanupErr, fmt.Errorf("rollback API index cleanup: %w", rollbackErr))
}

// rollbackRemovals restores each moved artifact without overwriting an uncooperative concurrent writer.
func (p *preparedCandidate) rollbackRemovals(removals []removal) error {
	var rollbackErr error
	for index := len(removals) - 1; index >= 0; index-- {
		removal := removals[index]
		if !removal.wasMoved {
			continue
		}
		latest, err := readFileSnapshot(removal.activePath)
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("inspect API index cleanup rollback path %q: %w", removal.activePath, err))
			continue
		}
		if latest.exists {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore stale API index artifact %q: path was recreated during cleanup", removal.activePath))
			continue
		}
		if err := p.renameArtifact(removal.tombstonePath, removal.activePath); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore stale API index artifact %q: %w", removal.activePath, err))
		}
	}
	return rollbackErr
}

// renameArtifact uses the operating-system rename unless a focused lifecycle test injects a failure.
func (p *preparedCandidate) renameArtifact(oldPath string, newPath string) error {
	if p.renameFile != nil {
		return p.renameFile(oldPath, newPath)
	}
	return os.Rename(oldPath, newPath)
}

// noteArtifactMutation exposes deterministic interleavings only to same-package lifecycle tests.
func (p *preparedCandidate) noteArtifactMutation(index int, path string) {
	if p.afterArtifactMutation != nil {
		p.afterArtifactMutation(index, path)
	}
}

// rollbackPublicationError preserves the triggering failure while reporting every rollback failure.
func rollbackPublicationError(published []publication, publicationErr error) error {
	rollbackErr := rollbackArtifacts(published)
	if rollbackErr == nil {
		return publicationErr
	}
	return errors.Join(publicationErr, fmt.Errorf("rollback API index publication: %w", rollbackErr))
}

// rollbackArtifacts restores the last complete active set when an unexpected rename fails mid-publication.
func rollbackArtifacts(artifacts []publication) error {
	var rollbackErr error
	for index := len(artifacts) - 1; index >= 0; index-- {
		artifact := artifacts[index]
		if !artifact.wasPublished {
			continue
		}
		if err := restoreFileSnapshot(artifact.activePath, artifact.active); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

// restoreFileSnapshot uses a sibling temporary file so rollback never exposes partially written JSON.
func restoreFileSnapshot(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove newly published API index artifact %q: %w", path, err)
		}
		return nil
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".forj-api-index-rollback-")
	if err != nil {
		return fmt.Errorf("create rollback artifact for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set rollback artifact permissions for %q: %w", path, err)
	}
	if _, err := temporary.Write(snapshot.content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write rollback artifact for %q: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync rollback artifact for %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close rollback artifact for %q: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("restore API index artifact %q: %w", path, err)
	}
	return nil
}

// Discard removes staged artifacts without changing the active generation and reports leaks callers may need to retry.
func (p *preparedCandidate) Discard() error {
	if p.stagingDir == "" {
		return nil
	}
	removeStagingDir := os.RemoveAll
	if p.removeStagingDir != nil {
		removeStagingDir = p.removeStagingDir
	}
	if err := removeStagingDir(p.stagingDir); err != nil {
		return fmt.Errorf("discard API index staging directory %q: %w", p.stagingDir, err)
	}
	p.stagingDir = ""
	return nil
}

// equal reports whether two complete artifact generations have identical presence and content.
func (s snapshots) equal(other snapshots) bool {
	return s.out.equal(other.out) &&
		s.diagnostics.equal(other.diagnostics) &&
		s.openAPI.equal(other.openAPI)
}

// anyExists distinguishes cleanup work from a CLI-only app that was already artifact-free.
func (s snapshots) anyExists() bool {
	return s.out.exists || s.diagnostics.exists || s.openAPI.exists
}

// equal reports whether two individual artifact snapshots are identical.
func (s fileSnapshot) equal(other fileSnapshot) bool {
	if s.exists != other.exists {
		return false
	}
	if !s.exists {
		return true
	}
	return bytes.Equal(s.content, other.content)
}

// readFileSnapshot treats absence as state while preserving every other filesystem failure.
func readFileSnapshot(path string) (fileSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}
	return fileSnapshot{content: content, exists: true}, nil
}
