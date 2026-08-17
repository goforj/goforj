//go:build !windows

package atlas

import "golang.org/x/sys/unix"

// availableEvaluationDiskBytes reports capacity on the volume that owns disposable evaluation work state.
func availableEvaluationDiskBytes(path string) (uint64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(path, &stats); err != nil {
		return 0, err
	}
	return stats.Bavail * uint64(stats.Bsize), nil
}
