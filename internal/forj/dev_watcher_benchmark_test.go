package forj

import "testing"

// BenchmarkDevWatcherBuildGateUncontended measures the coordinator overhead around one App build phase.
func BenchmarkDevWatcherBuildGateUncontended(b *testing.B) {
	gate := newDevWatcherBuildGate()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		gate.acquire(devWatcherBuildPhaseApp)
		gate.release(devWatcherBuildPhaseApp)
	}
}

// BenchmarkDevWatcherBuildGateParallelAppPhase measures the shared read-only phase under concurrent App builds.
func BenchmarkDevWatcherBuildGateParallelAppPhase(b *testing.B) {
	gate := newDevWatcherBuildGate()
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			gate.acquire(devWatcherBuildPhaseApp)
			gate.release(devWatcherBuildPhaseApp)
		}
	})
}
