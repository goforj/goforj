package devwatch

import (
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestDevWatchRuntimePresenceHelper keeps a real child alive for replacement-state tests.
func TestDevWatchRuntimePresenceHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEVWATCH_RUNTIME_PRESENCE_HELPER") != "1" {
		return
	}
	time.Sleep(time.Hour)
}

// TestPrepareExecutableCopiesValidatedArtifact verifies later source replacement cannot alter a prepared launch.
func TestPrepareExecutableCopiesValidatedArtifact(t *testing.T) {
	source := copyDevWatchHostExecutableFixture(t)
	prepared, err := PrepareExecutable(source)
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	if prepared.Path() == source {
		t.Fatal("PrepareExecutable() reused the publication path")
	}
	if err := os.WriteFile(source, []byte("replaced"), 0o755); err != nil {
		t.Fatalf("replace source executable: %v", err)
	}
	snapshot, err := os.ReadFile(prepared.Path())
	if err != nil {
		t.Fatalf("read prepared executable: %v", err)
	}
	if string(snapshot) == "replaced" || len(snapshot) < 1024 {
		t.Fatalf("prepared executable changed with source publication: size=%d", len(snapshot))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(prepared.Path())
		if err != nil {
			t.Fatalf("inspect prepared executable: %v", err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("prepared executable mode = %o, want 700", info.Mode().Perm())
		}
	}
}

// TestPrepareExecutableCanonicalizesRelativePath keeps command working directories from prefixing snapshots twice.
func TestPrepareExecutableCanonicalizesRelativePath(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	fixtureDirectory, err := os.MkdirTemp(workingDirectory, ".devwatch-relative-*")
	if err != nil {
		t.Fatalf("create same-volume executable fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(fixtureDirectory) })
	source := copyDevWatchHostExecutableFixtureTo(t, fixtureDirectory)
	relativeSource, err := filepath.Rel(workingDirectory, source)
	if err != nil {
		t.Fatalf("resolve relative executable path: %v", err)
	}
	prepared, err := PrepareExecutable(relativeSource)
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	if !filepath.IsAbs(prepared.Path()) {
		t.Fatalf("prepared executable path = %q, want absolute", prepared.Path())
	}
}

// TestPrepareExecutableRejectsUnsafeSources verifies preparation fails before returning a launchable artifact.
func TestPrepareExecutableRejectsUnsafeSources(t *testing.T) {
	directory := t.TempDir()
	testCases := []struct {
		name string
		path string
	}{
		{name: "missing", path: filepath.Join(directory, "missing")},
		{name: "directory", path: directory},
		{name: "invalid format", path: writeDevWatchExecutableFixture(t, []byte("not-an-executable"))},
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		testCases = append(testCases, struct {
			name string
			path string
		}{name: "truncated valid magic", path: writeDevWatchExecutableFixture(t, truncatedDevWatchExecutableHeader())})
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" ||
		runtime.GOOS == "netbsd" || runtime.GOOS == "dragonfly" || runtime.GOOS == "solaris" || runtime.GOOS == "illumos" {
		testCases = append(testCases,
			struct {
				name string
				path string
			}{name: "ELF object file", path: mutateDevWatchELFHeaderFixture(t, elf.ET_REL, 0)},
			struct {
				name string
				path string
			}{name: "foreign ELF architecture", path: mutateDevWatchELFHeaderFixture(t, 0, elf.EM_NONE)},
			struct {
				name string
				path string
			}{name: "ELF entry load beyond file", path: mutateDevWatchELFLoadRangeFixture(t)},
		)
	}
	if runtime.GOOS != "windows" {
		notExecutable := copyDevWatchHostExecutableFixture(t)
		if err := os.Chmod(notExecutable, 0o600); err != nil {
			t.Fatalf("remove executable mode: %v", err)
		}
		testCases = append(testCases, struct {
			name string
			path string
		}{name: "not executable", path: notExecutable})
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prepared, err := PrepareExecutable(testCase.path)
			if err == nil {
				_ = prepared.Cleanup()
				t.Fatalf("PrepareExecutable(%q) unexpectedly succeeded", testCase.path)
			}
		})
	}
}

// TestPreparedELFFormatDistinguishesSharedMachines verifies class and byte order remain part of host matching.
func TestPreparedELFFormatDistinguishesSharedMachines(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		architecture string
		class        elf.Class
		data         elf.Data
	}{
		{architecture: "386", class: elf.ELFCLASS32, data: elf.ELFDATA2LSB},
		{architecture: "amd64", class: elf.ELFCLASS64, data: elf.ELFDATA2LSB},
		{architecture: "mips", class: elf.ELFCLASS32, data: elf.ELFDATA2MSB},
		{architecture: "mipsle", class: elf.ELFCLASS32, data: elf.ELFDATA2LSB},
		{architecture: "mips64", class: elf.ELFCLASS64, data: elf.ELFDATA2MSB},
		{architecture: "mips64le", class: elf.ELFCLASS64, data: elf.ELFDATA2LSB},
		{architecture: "ppc64", class: elf.ELFCLASS64, data: elf.ELFDATA2MSB},
		{architecture: "ppc64le", class: elf.ELFCLASS64, data: elf.ELFDATA2LSB},
		{architecture: "s390x", class: elf.ELFCLASS64, data: elf.ELFDATA2MSB},
	}
	for _, testCase := range testCases {
		t.Run(testCase.architecture, func(t *testing.T) {
			class, data, ok := preparedELFFormat(testCase.architecture)
			if !ok || class != testCase.class || data != testCase.data {
				t.Fatalf(
					"preparedELFFormat(%q) = (%s, %s, %t), want (%s, %s, true)",
					testCase.architecture,
					class,
					data,
					ok,
					testCase.class,
					testCase.data,
				)
			}
		})
	}
}

// TestValidatePreparedELFRejectsLoaderInvalidPrograms covers non-entry load ranges and memory sizing.
func TestValidatePreparedELFRejectsLoaderInvalidPrograms(t *testing.T) {
	t.Parallel()
	const fileSize = uint64(0x400)
	if err := validatePreparedELF(preparedELFValidatorFixture(t), fileSize); err != nil {
		t.Fatalf("validatePreparedELF() valid fixture error = %v", err)
	}
	testCases := []struct {
		name   string
		mutate func(*elf.File)
	}{
		{
			name: "wrong class",
			mutate: func(file *elf.File) {
				if file.Class == elf.ELFCLASS64 {
					file.Class = elf.ELFCLASS32
				} else {
					file.Class = elf.ELFCLASS64
				}
			},
		},
		{
			name: "wrong byte order",
			mutate: func(file *elf.File) {
				if file.Data == elf.ELFDATA2LSB {
					file.Data = elf.ELFDATA2MSB
				} else {
					file.Data = elf.ELFDATA2LSB
				}
			},
		},
		{
			name: "non-entry load beyond file",
			mutate: func(file *elf.File) {
				file.Progs[1].Off = fileSize - 1
			},
		},
		{
			name: "load file size exceeds memory size",
			mutate: func(file *elf.File) {
				file.Progs[1].Memsz = file.Progs[1].Filesz - 1
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			file := preparedELFValidatorFixture(t)
			testCase.mutate(file)
			if err := validatePreparedELF(file, fileSize); err == nil {
				t.Fatal("validatePreparedELF() unexpectedly accepted malformed program")
			}
		})
	}
}

// TestValidatePreparedMachORejectsLoaderInvalidSegments covers every mapped segment rather than only executable text.
func TestValidatePreparedMachORejectsLoaderInvalidSegments(t *testing.T) {
	t.Parallel()
	const fileSize = uint64(0x200)
	if err := validatePreparedMachO(preparedMachOValidatorFixture(t), fileSize); err != nil {
		t.Fatalf("validatePreparedMachO() valid fixture error = %v", err)
	}
	testCases := []struct {
		name   string
		mutate func(*macho.File)
	}{
		{
			name: "wrong image width",
			mutate: func(file *macho.File) {
				if file.Magic == macho.Magic64 {
					file.Magic = macho.Magic32
				} else {
					file.Magic = macho.Magic64
				}
			},
		},
		{
			name: "non-entry segment beyond file",
			mutate: func(file *macho.File) {
				file.Loads[2].(*macho.Segment).Offset = fileSize - 1
			},
		},
		{
			name: "segment file size exceeds memory size",
			mutate: func(file *macho.File) {
				segment := file.Loads[2].(*macho.Segment)
				segment.Memsz = segment.Filesz - 1
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			file := preparedMachOValidatorFixture(t)
			testCase.mutate(file)
			if err := validatePreparedMachO(file, fileSize); err == nil {
				t.Fatal("validatePreparedMachO() unexpectedly accepted malformed segment")
			}
		})
	}
}

// TestValidatePreparedFatMachORejectsTruncatedSlice verifies declared architecture sizes stay inside the outer file.
func TestValidatePreparedFatMachORejectsTruncatedSlice(t *testing.T) {
	t.Parallel()
	file := preparedMachOValidatorFixture(t)
	fatFile := &macho.FatFile{Arches: []macho.FatArch{{
		FatArchHeader: macho.FatArchHeader{Cpu: file.Cpu, Offset: 0x100, Size: 0x200},
		File:          file,
	}}}
	if err := validatePreparedFatMachO(fatFile, 0x300); err != nil {
		t.Fatalf("validatePreparedFatMachO() valid fixture error = %v", err)
	}
	if err := validatePreparedFatMachO(fatFile, 0x2ff); err == nil {
		t.Fatal("validatePreparedFatMachO() unexpectedly accepted truncated architecture slice")
	}
	mismatchedCPU := macho.CpuAmd64
	if file.Cpu == mismatchedCPU {
		mismatchedCPU = macho.CpuArm64
	}
	fatFile.Arches[0].Cpu = mismatchedCPU
	if err := validatePreparedFatMachO(fatFile, 0x300); err == nil {
		t.Fatal("validatePreparedFatMachO() unexpectedly accepted a mismatched outer CPU")
	}
}

// TestPreparedMachOX8664UnixThreadEntryRejectsMalformedState verifies the fixed thread-state count and size.
func TestPreparedMachOX8664UnixThreadEntryRejectsMalformedState(t *testing.T) {
	t.Parallel()
	const wantEntry = uint64(0x1020304050607080)
	raw := make([]byte, 16+int(preparedMachOX8664ThreadWords)*4)
	binary.LittleEndian.PutUint32(raw[0:4], uint32(macho.LoadCmdUnixThread))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)))
	binary.LittleEndian.PutUint32(raw[8:12], preparedMachOX8664ThreadFlavor)
	binary.LittleEndian.PutUint32(raw[12:16], preparedMachOX8664ThreadWords)
	programCounterOffset := 16 + preparedMachOX8664ProgramCounter*4
	binary.LittleEndian.PutUint64(raw[programCounterOffset:programCounterOffset+8], wantEntry)
	entry, err := preparedMachOX8664UnixThreadEntry(binary.LittleEndian, raw)
	if err != nil || entry != wantEntry {
		t.Fatalf("preparedMachOX8664UnixThreadEntry() = (%#x, %v), want (%#x, nil)", entry, err, wantEntry)
	}

	wrongCount := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint32(wrongCount[12:16], preparedMachOX8664ThreadWords-1)
	if _, err := preparedMachOX8664UnixThreadEntry(binary.LittleEndian, wrongCount); err == nil {
		t.Fatal("preparedMachOX8664UnixThreadEntry() accepted a mismatched state count")
	}
	if _, err := preparedMachOX8664UnixThreadEntry(binary.LittleEndian, raw[:len(raw)-1]); err == nil {
		t.Fatal("preparedMachOX8664UnixThreadEntry() accepted a truncated state")
	}
}

// TestValidatePreparedPERejectsLoaderInvalidSectionsAndHeaderKind covers non-entry raw ranges and PE32 width.
func TestValidatePreparedPERejectsLoaderInvalidSectionsAndHeaderKind(t *testing.T) {
	t.Parallel()
	const fileSize = uint64(0x400)
	if err := validatePreparedPE(preparedPEValidatorFixture(t), fileSize); err != nil {
		t.Fatalf("validatePreparedPE() valid fixture error = %v", err)
	}
	testCases := []struct {
		name   string
		mutate func(*pe.File)
	}{
		{
			name: "wrong optional header width",
			mutate: func(file *pe.File) {
				if _, ok := file.OptionalHeader.(*pe.OptionalHeader64); ok {
					file.OptionalHeader = &pe.OptionalHeader32{AddressOfEntryPoint: 0x1010}
				} else {
					file.OptionalHeader = &pe.OptionalHeader64{AddressOfEntryPoint: 0x1010}
				}
			},
		},
		{
			name: "non-entry section beyond file",
			mutate: func(file *pe.File) {
				file.Sections[1].Offset = uint32(fileSize - 1)
			},
		},
		{
			name: "raw data without file offset",
			mutate: func(file *pe.File) {
				file.Sections[1].Offset = 0
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			file := preparedPEValidatorFixture(t)
			testCase.mutate(file)
			if err := validatePreparedPE(file, fileSize); err == nil {
				t.Fatal("validatePreparedPE() unexpectedly accepted malformed image")
			}
		})
	}
}

// TestPreparedRangeHelpersRejectOverflow verifies malformed headers cannot wrap range endpoints back into a file.
func TestPreparedRangeHelpersRejectOverflow(t *testing.T) {
	t.Parallel()
	maxUint64 := ^uint64(0)
	if preparedFileRangeValid(maxUint64-1, 4, maxUint64) {
		t.Fatal("preparedFileRangeValid() accepted an overflowing file range")
	}
	if preparedAddressInRange(0, maxUint64-1, 4) {
		t.Fatal("preparedAddressInRange() accepted an address below an overflowing range")
	}
}

// TestPreparedExecutableCleanupIsIdempotent verifies callers can safely release rejected command preparation.
func TestPreparedExecutableCleanupIsIdempotent(t *testing.T) {
	prepared, err := PrepareExecutable(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}
	if _, err := os.Stat(prepared.Path()); !os.IsNotExist(err) {
		t.Fatalf("prepared executable still exists after cleanup: %v", err)
	}
}

// TestPreparedExecutableCommandOwnershipCleansStartFailure verifies the supervisor releases artifacts it cannot start.
func TestPreparedExecutableCommandOwnershipCleansStartFailure(t *testing.T) {
	prepared, err := PrepareExecutable(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	command, err := prepared.Bind(Command{Args: []string{filepath.Join(t.TempDir(), "missing-command")}})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if _, err := supervisor.StartRuntime(context.Background(), "prepared", command); err == nil {
		t.Fatal("StartRuntime() unexpectedly succeeded")
	}
	if _, err := os.Stat(prepared.Path()); !os.IsNotExist(err) {
		t.Fatalf("start failure retained prepared executable: %v", err)
	}
}

// TestPreparedExecutableCommandOwnershipCleansProcessExit verifies normal reaping owns artifact cleanup.
func TestPreparedExecutableCommandOwnershipCleansProcessExit(t *testing.T) {
	prepared, err := PrepareExecutable(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	command, err := prepared.Bind(Command{Shell: "exit 0"})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	exit, err := supervisor.Run(context.Background(), "prepared", command)
	if err != nil || !exit.OK() {
		t.Fatalf("Run() exit = %+v, error = %v", exit, err)
	}
	if _, err := os.Stat(prepared.Path()); !os.IsNotExist(err) {
		t.Fatalf("process exit retained prepared executable: %v", err)
	}
}

// TestSupervisorRuntimeRunningTracksFailedReplacement verifies callers can retry after the old process is gone.
func TestSupervisorRuntimeRunningTracksFailedReplacement(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 100 * time.Millisecond})
	registerDevProcessSupervisorCleanup(t, supervisor)
	command := Command{
		Args: []string{os.Args[0], "-test.run=^TestDevWatchRuntimePresenceHelper$"},
		Env:  map[string]string{"GOFORJ_DEVWATCH_RUNTIME_PRESENCE_HELPER": "1"},
	}
	if _, err := supervisor.StartRuntime(context.Background(), "app", command); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if !supervisor.RuntimeRunning("app") {
		t.Fatal("RuntimeRunning() = false after successful start")
	}
	prepared, err := PrepareExecutable(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	failedCommand, err := prepared.Bind(Command{Args: []string{filepath.Join(t.TempDir(), "missing-command")}})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if _, err := supervisor.RestartRuntime(context.Background(), "app", failedCommand); err == nil {
		t.Fatal("RestartRuntime() unexpectedly succeeded")
	}
	if supervisor.RuntimeRunning("app") {
		t.Fatal("RuntimeRunning() retained the stopped runtime after replacement start failure")
	}
	if _, err := supervisor.StartRuntime(context.Background(), "app", command); err != nil {
		t.Fatalf("retry StartRuntime() error = %v", err)
	}
	if !supervisor.RuntimeRunning("app") {
		t.Fatal("RuntimeRunning() = false after successful retry")
	}
	if err := supervisor.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

// TestPreparedExecutableBindRejectsReuse ensures exactly one managed process owns snapshot cleanup.
func TestPreparedExecutableBindRejectsReuse(t *testing.T) {
	prepared, err := PrepareExecutable(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("PrepareExecutable() error = %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	if _, err := prepared.Bind(Command{Shell: "exit 0"}); err != nil {
		t.Fatalf("first Bind() error = %v", err)
	}
	if _, err := prepared.Bind(Command{Shell: "exit 0"}); err == nil {
		t.Fatal("second Bind() unexpectedly succeeded")
	}
}

// TestCreatePreparedExecutablePreservesWindowsSuffix keeps PE snapshots launchable by Windows file association rules.
func TestCreatePreparedExecutablePreservesWindowsSuffix(t *testing.T) {
	source := filepath.Join(t.TempDir(), "app.exe")
	snapshot, err := createPreparedExecutable(source)
	if err != nil {
		t.Fatalf("createPreparedExecutable() error = %v", err)
	}
	path := snapshot.Name()
	if err := snapshot.Close(); err != nil {
		t.Fatalf("close prepared executable fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	if filepath.Ext(path) != ".exe" {
		t.Fatalf("prepared executable extension = %q, want .exe", filepath.Ext(path))
	}
}

// copyDevWatchHostExecutableFixture copies the running test binary to a mutable publication path.
func copyDevWatchHostExecutableFixture(t *testing.T) string {
	t.Helper()
	return copyDevWatchHostExecutableFixtureTo(t, t.TempDir())
}

// copyDevWatchHostExecutableFixtureTo copies the running test binary into a caller-selected filesystem volume.
func copyDevWatchHostExecutableFixtureTo(t *testing.T, directory string) string {
	t.Helper()
	input, err := os.Open(devWatchHostExecutable(t))
	if err != nil {
		t.Fatalf("open host executable: %v", err)
	}
	defer input.Close()
	path := filepath.Join(directory, "app")
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatalf("create host executable fixture: %v", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatalf("copy host executable fixture: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close host executable fixture: %v", err)
	}
	return path
}

// devWatchHostExecutable resolves the complete host binary required by format parsers.
func devWatchHostExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve host executable: %v", err)
	}
	return path
}

// preparedELFValidatorFixture builds the minimum host-shaped program map needed by the loader checks.
func preparedELFValidatorFixture(t *testing.T) *elf.File {
	t.Helper()
	machine, machineOK := preparedELFMachine(runtime.GOARCH)
	class, data, formatOK := preparedELFFormat(runtime.GOARCH)
	if !machineOK || !formatOK {
		t.Skipf("ELF validator fixture does not support %s", runtime.GOARCH)
	}
	return &elf.File{
		FileHeader: elf.FileHeader{
			Class: class, Data: data, Type: elf.ET_EXEC, Machine: machine, Entry: 0x1010,
		},
		Progs: []*elf.Prog{
			{ProgHeader: elf.ProgHeader{
				Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_X,
				Off: 0x100, Vaddr: 0x1000, Filesz: 0x100, Memsz: 0x100,
			}},
			{ProgHeader: elf.ProgHeader{
				Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_W,
				Off: 0x200, Vaddr: 0x2000, Filesz: 0x100, Memsz: 0x180,
			}},
		},
	}
}

// preparedMachOValidatorFixture builds a thin host-CPU image with separate executable and data segments.
func preparedMachOValidatorFixture(t *testing.T) *macho.File {
	t.Helper()
	cpu, ok := preparedMachOCPU(runtime.GOARCH)
	magic, magicOK := preparedMachOMagic(runtime.GOARCH)
	if !ok || !magicOK {
		t.Skipf("Mach-O validator fixture does not support %s", runtime.GOARCH)
	}
	mainCommand := make([]byte, preparedMachOMainCommandSize)
	binary.LittleEndian.PutUint32(mainCommand[0:4], preparedMachOLoadCommandMain)
	binary.LittleEndian.PutUint32(mainCommand[4:8], uint32(len(mainCommand)))
	binary.LittleEndian.PutUint64(mainCommand[8:16], 0x50)
	return &macho.File{
		FileHeader: macho.FileHeader{Magic: magic, Cpu: cpu, Type: macho.TypeExec},
		ByteOrder:  binary.LittleEndian,
		Loads: []macho.Load{
			macho.LoadBytes(mainCommand),
			&macho.Segment{SegmentHeader: macho.SegmentHeader{
				Name: "__TEXT", Offset: 0x40, Filesz: 0x80, Memsz: 0x80,
				Prot: preparedMachOExecuteProtection,
			}},
			&macho.Segment{SegmentHeader: macho.SegmentHeader{
				Name: "__DATA", Offset: 0xc0, Filesz: 0x40, Memsz: 0x80,
			}},
			&macho.Segment{SegmentHeader: macho.SegmentHeader{
				Name: "__DWARF", Offset: 0x100, Filesz: 0x20,
			}},
		},
	}
}

// preparedPEValidatorFixture builds a host-machine image with one code section and one data section.
func preparedPEValidatorFixture(t *testing.T) *pe.File {
	t.Helper()
	machine, machineOK := preparedPEMachine(runtime.GOARCH)
	want64Bit, headerOK := preparedPEUses64BitOptionalHeader(runtime.GOARCH)
	if !machineOK || !headerOK {
		t.Skipf("PE validator fixture does not support %s", runtime.GOARCH)
	}
	var optionalHeader any = &pe.OptionalHeader32{AddressOfEntryPoint: 0x1010}
	if want64Bit {
		optionalHeader = &pe.OptionalHeader64{AddressOfEntryPoint: 0x1010}
	}
	return &pe.File{
		FileHeader: pe.FileHeader{
			Machine: machine, Characteristics: pe.IMAGE_FILE_EXECUTABLE_IMAGE,
		},
		OptionalHeader: optionalHeader,
		Sections: []*pe.Section{
			{SectionHeader: pe.SectionHeader{
				Name: ".text", VirtualSize: 0x100, VirtualAddress: 0x1000,
				Size: 0x100, Offset: 0x100, Characteristics: pe.IMAGE_SCN_MEM_EXECUTE,
			}},
			{SectionHeader: pe.SectionHeader{
				Name: ".data", VirtualSize: 0x100, VirtualAddress: 0x2000,
				Size: 0x100, Offset: 0x200,
			}},
		},
	}
}

// writeDevWatchExecutableFixture creates one host-shaped executable header for preparation tests.
func writeDevWatchExecutableFixture(t *testing.T, contents []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(path, contents, 0o755); err != nil {
		t.Fatalf("write executable fixture: %v", err)
	}
	return path
}

// truncatedDevWatchExecutableHeader returns valid magic without the complete format structures preparation requires.
func truncatedDevWatchExecutableHeader() []byte {
	switch runtime.GOOS {
	case "linux":
		return []byte{0x7f, 'E', 'L', 'F'}
	case "darwin":
		return []byte{0xcf, 0xfa, 0xed, 0xfe}
	case "windows":
		return []byte{'M', 'Z', 0, 0}
	default:
		return []byte("host")
	}
}

// mutateDevWatchELFHeaderFixture keeps a structurally complete image while making it non-launchable.
func mutateDevWatchELFHeaderFixture(t *testing.T, fileType elf.Type, machine elf.Machine) string {
	t.Helper()
	path := copyDevWatchHostExecutableFixture(t)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open ELF mutation fixture: %v", err)
	}
	defer file.Close()
	header := make([]byte, 20)
	if _, err := file.ReadAt(header, 0); err != nil {
		t.Fatalf("read ELF mutation header: %v", err)
	}
	var byteOrder binary.ByteOrder = binary.LittleEndian
	if elf.Data(header[5]) == elf.ELFDATA2MSB {
		byteOrder = binary.BigEndian
	}
	if fileType != 0 {
		byteOrder.PutUint16(header[16:18], uint16(fileType))
	}
	if machine != 0 || fileType == 0 {
		byteOrder.PutUint16(header[18:20], uint16(machine))
	}
	if _, err := file.WriteAt(header, 0); err != nil {
		t.Fatalf("write ELF mutation header: %v", err)
	}
	return path
}

// mutateDevWatchELFLoadRangeFixture keeps headers parseable while placing the executable load beyond EOF.
func mutateDevWatchELFLoadRangeFixture(t *testing.T) string {
	t.Helper()
	path := copyDevWatchHostExecutableFixture(t)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open ELF load-range fixture: %v", err)
	}
	defer file.Close()
	parsed, err := elf.NewFile(file)
	if err != nil {
		t.Fatalf("parse ELF load-range fixture: %v", err)
	}
	programIndex := -1
	for index, program := range parsed.Progs {
		if program.Type == elf.PT_LOAD && program.Flags&elf.PF_X != 0 &&
			preparedAddressInRange(parsed.Entry, program.Vaddr, program.Filesz) {
			programIndex = index
			break
		}
	}
	if programIndex < 0 {
		t.Fatal("host executable has no file-backed executable entry load")
	}
	header := make([]byte, 64)
	if _, err := file.ReadAt(header, 0); err != nil {
		t.Fatalf("read ELF file header: %v", err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("inspect ELF load-range fixture: %v", err)
	}
	var programOffset uint64
	var entrySize uint16
	var loadOffsetField int64
	switch parsed.Class {
	case elf.ELFCLASS64:
		programOffset = parsed.ByteOrder.Uint64(header[32:40])
		entrySize = parsed.ByteOrder.Uint16(header[54:56])
		loadOffsetField = 8
	case elf.ELFCLASS32:
		programOffset = uint64(parsed.ByteOrder.Uint32(header[28:32]))
		entrySize = parsed.ByteOrder.Uint16(header[42:44])
		loadOffsetField = 4
	default:
		t.Fatalf("unsupported ELF class %s", parsed.Class)
	}
	fieldOffset := int64(programOffset) + int64(programIndex)*int64(entrySize) + loadOffsetField
	invalidOffset := uint64(info.Size()) + 4096
	if parsed.Class == elf.ELFCLASS64 {
		value := make([]byte, 8)
		parsed.ByteOrder.PutUint64(value, invalidOffset)
		if _, err := file.WriteAt(value, fieldOffset); err != nil {
			t.Fatalf("write ELF64 invalid load offset: %v", err)
		}
	} else {
		value := make([]byte, 4)
		parsed.ByteOrder.PutUint32(value, uint32(invalidOffset))
		if _, err := file.WriteAt(value, fieldOffset); err != nil {
			t.Fatalf("write ELF32 invalid load offset: %v", err)
		}
	}
	if _, err := elf.NewFile(file); err != nil {
		t.Fatalf("mutated ELF fixture stopped parsing structurally: %v", err)
	}
	return path
}
