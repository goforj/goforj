//go:build unix

package projectupgrade

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestInspectionRejectsDirectoryFIFOReplacement covers both inventory and discovery after a successful directory precheck.
func TestInspectionRejectsDirectoryFIFOReplacement(t *testing.T) {
	for _, operation := range []string{"inventory", "discovery"} {
		t.Run(operation, func(t *testing.T) {
			directory := t.TempDir()
			name := "managed"
			if err := os.Mkdir(filepath.Join(directory, name), 0o700); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			before, err := checkedFileInfo(root, name)
			if err != nil || before == nil || !before.IsDir() {
				t.Fatalf("directory precheck: %v, %v", before, err)
			}
			if err := os.Rename(filepath.Join(directory, name), filepath.Join(directory, "held")); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(filepath.Join(directory, name), 0o600); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				if operation == "inventory" {
					finished <- preserveUnknown(root, name, nil, nil, &Plan{})
					return
				}
				_, err := fs.ReadDir(InspectionFS(root), name)
				finished <- err
			}()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("accepted FIFO as a directory")
				}
			case <-time.After(5 * time.Second):
				writer, err := syscall.Open(filepath.Join(directory, name), syscall.O_RDWR|syscall.O_NONBLOCK, 0)
				if err != nil {
					t.Fatalf("release blocked directory reader: %v", err)
				}
				defer syscall.Close(writer)
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("directory reader did not exit after opening a writer")
				}
				t.Fatal("inspection blocked opening a FIFO directory replacement")
			}
		})
	}
}

// TestInspectionFSConfinesMetadataAndOpens prevents symlink aliases from escaping the inspection root.
func TestInspectionFSConfinesMetadataAndOpens(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, outside, "source.go", "OUTSIDE")
	if err := os.Symlink(outside, filepath.Join(directory, "alias")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := InspectionFS(root)
	if file, err := files.Open("alias/source.go"); err == nil {
		file.Close()
		t.Fatal("opened outside root")
	}
	if _, err := fs.Stat(files, "alias/source.go"); err == nil {
		t.Fatal("read metadata outside root")
	}
	if err := unix.Mkfifo(filepath.Join(directory, "marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info, err := fs.Stat(files, "marker"); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO marker metadata: %v, %v", info, err)
	}
}

// TestOpenRegularFileRejectsFIFOReplacement fixes the inspection boundary at the exact precheck-to-open race.
func TestOpenRegularFileRejectsFIFOReplacement(t *testing.T) {
	directory := t.TempDir()
	name := "managed"
	writeFixture(t, directory, name, "SAFE")
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before, err := checkedFileInfo(root, name)
	if err != nil || before == nil {
		t.Fatalf("precheck: %v", err)
	}
	fifo := filepath.Join(directory, "replacement")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fifo, filepath.Join(directory, name)); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		file, _, err := openRegularFile(root, name, before)
		if file != nil {
			file.Close()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "file changed while opening") {
			t.Fatalf("FIFO replacement error: %v", err)
		}
	case <-time.After(5 * time.Second):
		// A writer releases a regressed blocking open so the test can clean up its rooted handle.
		writer, err := syscall.Open(filepath.Join(directory, name), syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatalf("release blocked FIFO reader: %v", err)
		}
		defer syscall.Close(writer)
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("FIFO reader did not exit after opening a writer")
		}
		t.Fatal("inspection blocked opening a FIFO replacement")
	}
}

// TestOpenInspectionFileReadsReadOnlyRegularFile verifies that nonblocking opens retain read-only regular-file behavior.
func TestOpenInspectionFileReadsReadOnlyRegularFile(t *testing.T) {
	directory := t.TempDir()
	name := "managed"
	writeFixture(t, directory, name, "SAFE")
	if err := os.Chmod(filepath.Join(directory, name), 0o400); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := openInspectionFile(root, name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "SAFE" {
		t.Fatalf("regular file read: %q, %v", data, err)
	}
	if _, err := file.Write([]byte("CHANGED")); err == nil {
		t.Fatal("inspection descriptor accepted a write")
	}
	data, _, err = readRegular(root, name)
	if err != nil || string(data) != "SAFE" {
		t.Fatalf("regular file inspection: %q, %v", data, err)
	}
}
