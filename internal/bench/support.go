package bench

import "github.com/goforj/goforj/internal/testkit"

// repoForjExecutable builds the current CLI so benchmarks exercise uncommitted source changes.
func repoForjExecutable(modCache, buildCache string) (string, func(), error) {
	return testkit.BuildForjBinary(modCache, buildCache)
}
