package scenarios

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scenarioWriteCloser exposes independent write and close failures for file-lifecycle tests.
type scenarioWriteCloser struct {
	writeErr error
	closeErr error
	closes   int
}

// Write returns the selected failure without involving host filesystem buffering.
func (file *scenarioWriteCloser) Write(content []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return len(content), nil
}

// Close records lifecycle ownership before returning the selected delayed failure.
func (file *scenarioWriteCloser) Close() error {
	file.closes++
	return file.closeErr
}

// TestScenarioWorkspaceCleanupAfterReportsCleanupWithoutMaskingRunFailure verifies teardown remains observable on both success and failure paths.
func TestScenarioWorkspaceCleanupAfterReportsCleanupWithoutMaskingRunFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write cleanup blocker: %v", err)
	}
	workspace := scenarioWorkspace{
		root:        filepath.Join(blocker, "workspace"),
		removeAfter: true,
	}
	primaryErr := errors.New("scenario failed")
	for _, test := range []struct {
		name    string
		primary error
	}{
		{name: "cleanup only"},
		{name: "primary and cleanup", primary: primaryErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := workspace.cleanupAfter(test.primary)
			if test.primary != nil && !errors.Is(err, test.primary) {
				t.Fatalf("cleanupAfter() error = %v, want primary scenario failure", err)
			}
			if !strings.Contains(err.Error(), "remove temporary scenario workspace") {
				t.Fatalf("cleanupAfter() error = %v, want cleanup failure", err)
			}
		})
	}
}

// TestAppendAndCloseScenarioFileReportsEveryFailure verifies delayed close failures remain visible alone and beside write failures.
func TestAppendAndCloseScenarioFileReportsEveryFailure(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name     string
		writeErr error
		want     []error
	}{
		{name: "close failure", want: []error{closeErr}},
		{name: "write and close failures", writeErr: writeErr, want: []error{writeErr, closeErr}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := &scenarioWriteCloser{writeErr: test.writeErr, closeErr: closeErr}
			err := appendAndCloseScenarioFile(file, []byte("content"))

			for _, want := range test.want {
				if !errors.Is(err, want) {
					t.Fatalf("appendAndCloseScenarioFile() error = %v, want %v", err, want)
				}
			}
			if file.closes != 1 {
				t.Fatalf("Close() calls = %d, want 1", file.closes)
			}
		})
	}
}
