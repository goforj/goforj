package generate

import (
	"fmt"
	"os"
)

var cacheLegacyGeneratedFiles = []string{
	"runtime.go",
	"manager.go",
	"stores_gen.go",
	"config_gen.go",
}

var eventLegacyGeneratedFiles = []string{
	"driver.go",
	"driver_gen.go",
	"factory.go",
	"bus_redis.go",
	"bus_inproc.go",
	"helpers.go",
	"driver_test.go",
	"factory_test.go",
	"bus_redis_test.go",
	"bus_inproc_test.go",
	"helpers_test.go",
}

var mailLegacyGeneratedFiles = []string{
	"manager.go",
}

var queueLegacyGeneratedFiles = []string{
	"runtime.go",
	"manager.go",
	"queues_gen.go",
	"config_gen.go",
}

var storageLegacyGeneratedFiles = []string{
	"runtime.go",
	"manager.go",
	"disks_gen.go",
	"config_gen.go",
}

// removeGeneratedFileIfExists makes legacy cleanup observable without treating an already-migrated project as a failure.
func removeGeneratedFileIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove legacy generated file %q: %w", path, err)
	}
	return true, nil
}
