package bench

import "testing"

// TestParseHTTPRuntimeBenchmarkOutputNamesMetrics verifies benchmark columns retain their units after parsing.
func TestParseHTTPRuntimeBenchmarkOutputNamesMetrics(t *testing.T) {
	stdout := "BenchmarkHTTPRuntimeModes/health_route/baseline-12  1000  125.5 ns/op  64 B/op  3 allocs/op\n"
	got, err := parseHTTPRuntimeBenchmarkOutput(stdout, "baseline")
	if err != nil {
		t.Fatalf("parse benchmark output: %v", err)
	}
	want := httpRuntimeBenchmarkMetrics{nsPerOp: 125.5, bytesPerOp: 64, allocsPerOp: 3}
	if got != want {
		t.Fatalf("benchmark metrics = %#v, want %#v", got, want)
	}
}
