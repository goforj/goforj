package devwatch

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// PreparedExecutable is an immutable private copy that can outlive publication of its source path.
type PreparedExecutable struct {
	path        string
	mu          sync.Mutex
	bound       bool
	cleaned     bool
	cleanupOnce sync.Once
	cleanupErr  error
}

// PrepareExecutable copies and validates an executable before a live runtime replacement begins.
func PrepareExecutable(path string) (*PreparedExecutable, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("prepare executable: path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: resolve absolute path: %w", path, err)
	}
	path = absolutePath
	source, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: %w", path, err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: inspect source: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("prepare executable %q: source is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("prepare executable %q: source is not executable", path)
	}

	snapshot, err := createPreparedExecutable(path)
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: create snapshot: %w", path, err)
	}
	snapshotPath := snapshot.Name()
	keepSnapshot := false
	defer func() {
		_ = snapshot.Close()
		if !keepSnapshot {
			_ = os.Remove(snapshotPath)
		}
	}()
	copied, err := io.Copy(snapshot, source)
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: copy snapshot: %w", path, err)
	}
	afterCopy, err := source.Stat()
	if err != nil {
		return nil, fmt.Errorf("prepare executable %q: inspect source after copy: %w", path, err)
	}
	if copied != info.Size() || afterCopy.Size() != info.Size() ||
		!afterCopy.ModTime().Equal(info.ModTime()) || afterCopy.Mode() != info.Mode() {
		return nil, fmt.Errorf("prepare executable %q: source changed while it was copied", path)
	}
	if err := snapshot.Chmod(0o700); err != nil {
		return nil, fmt.Errorf("prepare executable %q: secure snapshot: %w", path, err)
	}
	if err := snapshot.Sync(); err != nil {
		return nil, fmt.Errorf("prepare executable %q: sync snapshot: %w", path, err)
	}
	if err := validatePreparedExecutable(snapshot); err != nil {
		return nil, fmt.Errorf("prepare executable %q: %w", path, err)
	}
	if err := snapshot.Close(); err != nil {
		return nil, fmt.Errorf("prepare executable %q: close snapshot: %w", path, err)
	}
	keepSnapshot = true
	return &PreparedExecutable{path: snapshotPath}, nil
}

// Path returns the private executable path selected for one runtime start.
func (p *PreparedExecutable) Path() string {
	return p.path
}

// Cleanup removes the private executable exactly once after start failure or process exit.
func (p *PreparedExecutable) Cleanup() error {
	p.cleanupOnce.Do(func() {
		p.mu.Lock()
		p.cleaned = true
		p.mu.Unlock()
		err := os.Remove(p.path)
		if err != nil && !os.IsNotExist(err) {
			p.cleanupErr = fmt.Errorf("remove prepared executable %q: %w", p.path, err)
		}
	})
	return p.cleanupErr
}

// Bind transfers cleanup of the prepared executable to one command lifecycle.
func (p *PreparedExecutable) Bind(command Command) (Command, error) {
	p.mu.Lock()
	if p.bound || p.cleaned {
		p.mu.Unlock()
		return Command{}, errors.New("prepared executable lifecycle is already owned or cleaned")
	}
	p.bound = true
	p.mu.Unlock()
	previousCleanup := command.cleanup
	command.cleanup = func() error {
		var previousErr error
		if previousCleanup != nil {
			previousErr = previousCleanup()
		}
		return errors.Join(previousErr, p.Cleanup())
	}
	return command, nil
}

// createPreparedExecutable keeps the snapshot beside its source so both paths share execution permissions.
func createPreparedExecutable(sourcePath string) (*os.File, error) {
	directory := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	pattern := "." + stem + ".run-*" + extension
	return os.CreateTemp(directory, pattern)
}

// validatePreparedExecutable rejects partial or foreign artifacts before the previous runtime is disturbed.
func validatePreparedExecutable(snapshot *os.File) error {
	info, err := snapshot.Stat()
	if err != nil {
		return fmt.Errorf("inspect prepared executable: %w", err)
	}
	fileSize := uint64(info.Size())
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "illumos":
		file, err := elf.NewFile(snapshot)
		if err != nil {
			return fmt.Errorf("snapshot is not a complete ELF executable: %w", err)
		}
		if err := validatePreparedELF(file, fileSize); err != nil {
			return err
		}
	case "darwin":
		file, thinErr := macho.NewFile(snapshot)
		if thinErr == nil {
			if err := validatePreparedMachO(file, fileSize); err != nil {
				return err
			}
		} else {
			fatFile, fatErr := macho.NewFatFile(snapshot)
			if fatErr != nil {
				return fmt.Errorf("snapshot is not a complete Mach-O executable: %w", errors.Join(thinErr, fatErr))
			}
			if err := validatePreparedFatMachO(fatFile, fileSize); err != nil {
				return err
			}
		}
	case "windows":
		file, err := pe.NewFile(snapshot)
		if err != nil {
			return fmt.Errorf("snapshot is not a complete PE executable: %w", err)
		}
		if err := validatePreparedPE(file, fileSize); err != nil {
			return err
		}
	default:
		return fmt.Errorf("executable validation is unsupported on %s", runtime.GOOS)
	}
	return nil
}

// validatePreparedELF requires a host-architecture executable image rather than a linkable object or library.
func validatePreparedELF(file *elf.File, fileSize uint64) error {
	if file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN {
		return fmt.Errorf("snapshot ELF type %s is not executable", file.Type)
	}
	if file.Entry == 0 {
		return errors.New("snapshot ELF executable has no entry point")
	}
	want, ok := preparedELFMachine(runtime.GOARCH)
	if !ok || file.Machine != want {
		return fmt.Errorf("snapshot ELF machine %s does not match %s", file.Machine, runtime.GOARCH)
	}
	wantClass, wantData, ok := preparedELFFormat(runtime.GOARCH)
	if !ok || file.Class != wantClass || file.Data != wantData {
		return fmt.Errorf(
			"snapshot ELF format %s/%s does not match %s",
			file.Class,
			file.Data,
			runtime.GOARCH,
		)
	}
	entryInExecutableLoad := false
	for _, program := range file.Progs {
		if !preparedFileRangeValid(program.Off, program.Filesz, fileSize) {
			return fmt.Errorf("snapshot ELF program %s extends beyond the file", program.Type)
		}
		if program.Type != elf.PT_LOAD {
			continue
		}
		if program.Filesz > program.Memsz {
			return errors.New("snapshot ELF load segment has more file data than memory space")
		}
		if program.Flags&elf.PF_X != 0 && preparedAddressInRange(file.Entry, program.Vaddr, program.Filesz) {
			entryInExecutableLoad = true
		}
	}
	if !entryInExecutableLoad {
		return errors.New("snapshot ELF entry point is not in a file-backed executable load segment")
	}
	return nil
}

// preparedELFMachine maps supported Go architectures to their loader machine identifier.
func preparedELFMachine(architecture string) (elf.Machine, bool) {
	switch architecture {
	case "386":
		return elf.EM_386, true
	case "amd64":
		return elf.EM_X86_64, true
	case "arm":
		return elf.EM_ARM, true
	case "arm64":
		return elf.EM_AARCH64, true
	case "loong64":
		return elf.EM_LOONGARCH, true
	case "mips", "mipsle", "mips64", "mips64le":
		return elf.EM_MIPS, true
	case "ppc64", "ppc64le":
		return elf.EM_PPC64, true
	case "riscv64":
		return elf.EM_RISCV, true
	case "s390x":
		return elf.EM_S390, true
	default:
		return elf.EM_NONE, false
	}
}

// preparedELFFormat distinguishes architectures that share a machine identifier but not a loader ABI.
func preparedELFFormat(architecture string) (elf.Class, elf.Data, bool) {
	switch architecture {
	case "386", "arm", "mipsle":
		return elf.ELFCLASS32, elf.ELFDATA2LSB, true
	case "mips":
		return elf.ELFCLASS32, elf.ELFDATA2MSB, true
	case "amd64", "arm64", "loong64", "mips64le", "ppc64le", "riscv64":
		return elf.ELFCLASS64, elf.ELFDATA2LSB, true
	case "mips64", "ppc64", "s390x":
		return elf.ELFCLASS64, elf.ELFDATA2MSB, true
	default:
		return elf.ELFCLASSNONE, elf.ELFDATANONE, false
	}
}

// validatePreparedFatMachO proves every declared image is physically present before selecting the host slice.
func validatePreparedFatMachO(file *macho.FatFile, fileSize uint64) error {
	matched := false
	for _, architecture := range file.Arches {
		if !preparedFileRangeValid(uint64(architecture.Offset), uint64(architecture.Size), fileSize) {
			return fmt.Errorf(
				"snapshot Mach-O image for CPU %s extends beyond the fat file",
				architecture.Cpu,
			)
		}
		if architecture.Cpu != architecture.File.Cpu || architecture.SubCpu != architecture.File.SubCpu {
			return errors.New("snapshot Mach-O fat header does not match its embedded image")
		}
		if validatePreparedMachO(architecture.File, uint64(architecture.Size)) == nil {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("snapshot has no executable Mach-O image for %s", runtime.GOARCH)
	}
	return nil
}

// validatePreparedMachO requires a launchable thin image matching the current process architecture.
func validatePreparedMachO(file *macho.File, fileSize uint64) error {
	if file.Type != macho.TypeExec {
		return fmt.Errorf("snapshot Mach-O type %s is not executable", file.Type)
	}
	want, ok := preparedMachOCPU(runtime.GOARCH)
	if !ok || file.Cpu != want {
		return fmt.Errorf("snapshot Mach-O CPU %s does not match %s", file.Cpu, runtime.GOARCH)
	}
	wantMagic, ok := preparedMachOMagic(runtime.GOARCH)
	if !ok || file.Magic != wantMagic {
		return fmt.Errorf("snapshot Mach-O image width does not match %s", runtime.GOARCH)
	}
	entry, fileOffset, entryIsFileOffset, err := preparedMachOEntry(file)
	if err != nil {
		return err
	}
	entryInExecutableSegment := false
	for _, load := range file.Loads {
		segment, ok := load.(*macho.Segment)
		if !ok {
			continue
		}
		if !preparedFileRangeValid(segment.Offset, segment.Filesz, fileSize) {
			return fmt.Errorf("snapshot Mach-O segment %q extends beyond the file", segment.Name)
		}
		// Go's __DWARF segment deliberately stores file data without mapping memory at launch.
		loaderMapped := segment.Memsz > 0 || segment.Prot != 0 || segment.Maxprot != 0
		if loaderMapped && segment.Filesz > segment.Memsz {
			return fmt.Errorf("snapshot Mach-O segment %q has more file data than memory space", segment.Name)
		}
		if segment.Prot&preparedMachOExecuteProtection == 0 {
			continue
		}
		if entryIsFileOffset {
			if preparedAddressInRange(fileOffset, segment.Offset, segment.Filesz) {
				entryInExecutableSegment = true
			}
			continue
		}
		if preparedAddressInRange(entry, segment.Addr, segment.Filesz) {
			entryInExecutableSegment = true
		}
	}
	if !entryInExecutableSegment {
		return errors.New("snapshot Mach-O entry point is not in a file-backed executable segment")
	}
	return nil
}

// preparedMachOCPU maps supported Darwin Go architectures to Mach-O CPU identifiers.
func preparedMachOCPU(architecture string) (macho.Cpu, bool) {
	switch architecture {
	case "386":
		return macho.Cpu386, true
	case "amd64":
		return macho.CpuAmd64, true
	case "arm":
		return macho.CpuArm, true
	case "arm64":
		return macho.CpuArm64, true
	case "ppc":
		return macho.CpuPpc, true
	case "ppc64":
		return macho.CpuPpc64, true
	default:
		return 0, false
	}
}

// preparedMachOMagic maps the host architecture to the thin header width its loader accepts.
func preparedMachOMagic(architecture string) (uint32, bool) {
	switch architecture {
	case "386", "arm", "ppc":
		return macho.Magic32, true
	case "amd64", "arm64", "ppc64":
		return macho.Magic64, true
	default:
		return 0, false
	}
}

// validatePreparedPE rejects object files and DLLs even when their headers parse successfully.
func validatePreparedPE(file *pe.File, fileSize uint64) error {
	if file.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || file.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return errors.New("snapshot PE image is not a standalone executable")
	}
	want, ok := preparedPEMachine(runtime.GOARCH)
	if !ok || file.Machine != want {
		return fmt.Errorf("snapshot PE machine %#x does not match %s", file.Machine, runtime.GOARCH)
	}
	var entry uint32
	want64Bit, ok := preparedPEUses64BitOptionalHeader(runtime.GOARCH)
	if !ok {
		return fmt.Errorf("snapshot PE optional header architecture is unsupported on %s", runtime.GOARCH)
	}
	switch optional := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if want64Bit {
			return fmt.Errorf("snapshot PE32 optional header does not match %s", runtime.GOARCH)
		}
		entry = optional.AddressOfEntryPoint
	case *pe.OptionalHeader64:
		if !want64Bit {
			return fmt.Errorf("snapshot PE32+ optional header does not match %s", runtime.GOARCH)
		}
		entry = optional.AddressOfEntryPoint
	default:
		return errors.New("snapshot PE executable has no optional image header")
	}
	if entry == 0 {
		return errors.New("snapshot PE executable has no entry point")
	}
	entryInExecutableSection := false
	for _, section := range file.Sections {
		if section.Size > 0 && section.Offset == 0 {
			return fmt.Errorf("snapshot PE section %q has file data without a file offset", section.Name)
		}
		if !preparedFileRangeValid(uint64(section.Offset), uint64(section.Size), fileSize) {
			return fmt.Errorf("snapshot PE section %q extends beyond the file", section.Name)
		}
		if section.Characteristics&pe.IMAGE_SCN_MEM_EXECUTE == 0 {
			continue
		}
		if entry < section.VirtualAddress {
			continue
		}
		delta := entry - section.VirtualAddress
		virtualSize := section.VirtualSize
		if virtualSize == 0 {
			virtualSize = section.Size
		}
		if delta >= virtualSize || delta >= section.Size {
			continue
		}
		entryInExecutableSection = true
	}
	if !entryInExecutableSection {
		return errors.New("snapshot PE entry point is not in a file-backed executable section")
	}
	return nil
}

// preparedPEMachine maps supported Windows Go architectures to PE machine identifiers.
func preparedPEMachine(architecture string) (uint16, bool) {
	switch architecture {
	case "386":
		return pe.IMAGE_FILE_MACHINE_I386, true
	case "amd64":
		return pe.IMAGE_FILE_MACHINE_AMD64, true
	case "arm":
		return pe.IMAGE_FILE_MACHINE_ARMNT, true
	case "arm64":
		return pe.IMAGE_FILE_MACHINE_ARM64, true
	default:
		return 0, false
	}
}

// preparedPEUses64BitOptionalHeader maps the host architecture to the PE image header required by its loader.
func preparedPEUses64BitOptionalHeader(architecture string) (bool, bool) {
	switch architecture {
	case "386", "arm":
		return false, true
	case "amd64", "arm64":
		return true, true
	default:
		return false, false
	}
}

const (
	preparedMachOLoadCommandMain     uint32 = 0x80000028
	preparedMachOExecuteProtection   uint32 = 0x4
	preparedMachOX8664ThreadFlavor   uint32 = 4
	preparedMachOX8664ThreadWords    uint32 = 42
	preparedMachOX8664ProgramCounter        = 32
	preparedMachOMainCommandSize            = 24
)

// preparedMachOEntry derives the loader entry from LC_MAIN or Go's AMD64 LC_UNIXTHREAD command.
func preparedMachOEntry(file *macho.File) (uint64, uint64, bool, error) {
	for _, load := range file.Loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}
		command := file.ByteOrder.Uint32(raw[:4])
		switch command {
		case preparedMachOLoadCommandMain:
			if len(raw) != preparedMachOMainCommandSize {
				return 0, 0, false, errors.New("snapshot Mach-O LC_MAIN command has an invalid size")
			}
			return 0, file.ByteOrder.Uint64(raw[8:16]), true, nil
		case uint32(macho.LoadCmdUnixThread):
			if runtime.GOARCH != "amd64" {
				continue
			}
			entry, err := preparedMachOX8664UnixThreadEntry(file.ByteOrder, raw)
			return entry, 0, false, err
		}
	}
	return 0, 0, false, errors.New("snapshot Mach-O executable has no supported entry command")
}

// preparedMachOX8664UnixThreadEntry validates the fixed x86 thread-state payload before reading its program counter.
func preparedMachOX8664UnixThreadEntry(byteOrder binary.ByteOrder, raw []byte) (uint64, error) {
	expectedLength := 16 + int(preparedMachOX8664ThreadWords)*4
	if len(raw) != expectedLength ||
		byteOrder.Uint32(raw[8:12]) != preparedMachOX8664ThreadFlavor ||
		byteOrder.Uint32(raw[12:16]) != preparedMachOX8664ThreadWords {
		return 0, errors.New("snapshot Mach-O LC_UNIXTHREAD command is incomplete")
	}
	programCounterOffset := 16 + preparedMachOX8664ProgramCounter*4
	return byteOrder.Uint64(raw[programCounterOffset : programCounterOffset+8]), nil
}

// preparedAddressInRange compares unsigned address ranges without overflow.
func preparedAddressInRange(address uint64, start uint64, size uint64) bool {
	return address >= start && address-start < size
}

// preparedFileRangeValid rejects truncated segments without overflowing offset arithmetic.
func preparedFileRangeValid(offset uint64, size uint64, fileSize uint64) bool {
	return offset <= fileSize && size <= fileSize-offset
}
