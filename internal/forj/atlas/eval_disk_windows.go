//go:build windows

package atlas

import "golang.org/x/sys/windows"

// availableEvaluationDiskBytes reports capacity on the volume that owns disposable evaluation work state.
func availableEvaluationDiskBytes(path string) (uint64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, nil, nil); err != nil {
		return 0, err
	}
	return available, nil
}
