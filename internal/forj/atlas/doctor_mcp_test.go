package atlas

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goforj/atlas/install"
)

// TestProbeMCPReportsMissingExecutable covers the configured command lookup failure.
func TestProbeMCPReportsMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result := probeMCP(t.Context(), t.TempDir())
	if result.Executable || result.Err == nil || !strings.Contains(result.Err.Error(), "find forj executable") {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommandReportsStartFailure covers a resolved command that cannot be started.
func TestProbeMCPCommandReportsStartFailure(t *testing.T) {
	result := probeMCPCommand(t.Context(), t.TempDir(), nil, t.TempDir())
	if !result.Executable || result.Err == nil || !strings.Contains(result.Err.Error(), "start Atlas MCP") {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommand verifies the complete initialization and tool-list readiness path.
func TestProbeMCPCommand(t *testing.T) {
	t.Setenv("GOFORJ_MCP_PROBE_HELPER", "ready")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := probeMCPCommand(ctx, os.Args[0], []string{"-test.run=^TestMCPProbeHelper$"}, t.TempDir())
	if !result.Executable || !result.Protocol || result.Tools != 2 || !result.Ready || result.Err != nil {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommandRejectsMissingApplicationInfo covers the required-tool failure branch.
func TestProbeMCPCommandRejectsMissingApplicationInfo(t *testing.T) {
	t.Setenv("GOFORJ_MCP_PROBE_HELPER", "missing-tool")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := probeMCPCommand(ctx, os.Args[0], []string{"-test.run=^TestMCPProbeHelper$"}, t.TempDir())
	if result.Ready || result.Err == nil || !strings.Contains(result.Err.Error(), "application-info") {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommandReportsMalformedProtocol covers invalid server output without hiding the decode failure.
func TestProbeMCPCommandReportsMalformedProtocol(t *testing.T) {
	t.Setenv("GOFORJ_MCP_PROBE_HELPER", "malformed")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := probeMCPCommand(ctx, os.Args[0], []string{"-test.run=^TestMCPProbeHelper$"}, t.TempDir())
	if result.Protocol || result.Err == nil || !strings.Contains(result.Err.Error(), "decode JSON-RPC") {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommandReportsTimeout covers a server that never completes initialization.
func TestProbeMCPCommandReportsTimeout(t *testing.T) {
	t.Setenv("GOFORJ_MCP_PROBE_HELPER", "timeout")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	result := probeMCPCommand(ctx, os.Args[0], []string{"-test.run=^TestMCPProbeHelper$"}, t.TempDir())
	if result.Ready || result.Err == nil || !strings.Contains(result.Err.Error(), "deadline exceeded") {
		t.Fatalf("probe result = %#v", result)
	}
}

// TestProbeMCPCommandReportsProtocolFailures covers server-declared and incomplete initialization responses.
func TestProbeMCPCommandReportsProtocolFailures(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		want     string
	}{
		{name: "initialize error", scenario: "initialize-error", want: "JSON-RPC -32000"},
		{name: "missing protocol", scenario: "missing-protocol", want: "did not negotiate"},
		{name: "tools error", scenario: "tools-error", want: "JSON-RPC -32001"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GOFORJ_MCP_PROBE_HELPER", test.scenario)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			result := probeMCPCommand(ctx, os.Args[0], []string{"-test.run=^TestMCPProbeHelper$"}, t.TempDir())
			if result.Ready || result.Err == nil || !strings.Contains(result.Err.Error(), test.want) {
				t.Fatalf("probe result = %#v", result)
			}
		})
	}
}

// TestReadMCPProbeResponseRejectsOversizedMessages covers the response-size boundary.
func TestReadMCPProbeResponseRejectsOversizedMessages(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(strings.Repeat("x", (1<<20)+1) + "\n"))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	if _, err := readMCPProbeResponse(scanner); err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("oversized response error = %v", err)
	}
}

// TestMCPProbeMessageIOFailures covers closed request streams and empty response streams.
func TestMCPProbeMessageIOFailures(t *testing.T) {
	reader, writer := io.Pipe()
	if err := reader.Close(); err != nil {
		t.Fatalf("close request reader: %v", err)
	}
	if err := writeMCPProbeMessage(writer, map[string]any{"jsonrpc": "2.0"}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed request stream error = %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(""))
	if _, err := readMCPProbeResponse(scanner); !errors.Is(err, io.EOF) {
		t.Fatalf("empty response stream error = %v", err)
	}
}

// TestMCPProbeFailureSelectsActionableEvidence covers timeout, stderr, and plain read failures.
func TestMCPProbeFailureSelectsActionableEvidence(t *testing.T) {
	if got := mcpProbeFailure("initialize", io.EOF, "ignored", context.DeadlineExceeded); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("context failure = %v", got)
	}
	if got := mcpProbeFailure("initialize", io.EOF, "server detail", nil); !strings.Contains(got.Error(), "server detail") {
		t.Fatalf("stderr failure = %v", got)
	}
	if got := mcpProbeFailure("initialize", io.EOF, strings.Repeat("x", 600), nil); strings.Count(got.Error(), "x") != 500 {
		t.Fatalf("bounded stderr failure length = %d", strings.Count(got.Error(), "x"))
	}
	if got := mcpProbeFailure("initialize", io.EOF, "", nil); !errors.Is(got, io.EOF) {
		t.Fatalf("read failure = %v", got)
	}
}

// TestMCPProbeHelper acts as a deterministic stdio MCP server for subprocess probe tests.
func TestMCPProbeHelper(t *testing.T) {
	scenario := os.Getenv("GOFORJ_MCP_PROBE_HELPER")
	if scenario == "" {
		return
	}
	reader := bufio.NewScanner(os.Stdin)
	if !reader.Scan() {
		return
	}
	if scenario == "timeout" {
		time.Sleep(time.Second)
		return
	}
	if scenario == "malformed" {
		fmt.Fprintln(os.Stdout, "not-json")
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	initialize := map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "test", "version": "1"}}}
	if scenario == "initialize-error" {
		initialize = map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -32000, "message": "initialization failed"}}
	}
	if scenario == "missing-protocol" {
		initialize = map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}}
	}
	if err := encoder.Encode(initialize); err != nil {
		t.Fatalf("write initialize response: %v", err)
	}
	if scenario == "initialize-error" || scenario == "missing-protocol" {
		return
	}
	if !reader.Scan() || !reader.Scan() {
		return
	}
	if scenario == "tools-error" {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "error": map[string]any{"code": -32001, "message": "tools unavailable"}}); err != nil {
			t.Fatalf("write tools error: %v", err)
		}
		return
	}
	tools := []map[string]any{{"name": "route-list"}, {"name": "application-info"}}
	if scenario == "missing-tool" {
		tools = tools[:1]
	}
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": tools}}); err != nil {
		t.Fatalf("write tools response: %v", err)
	}
}

// TestPrintActivationHintLimitsRestartGuidanceToCodex verifies other adapters do not receive Codex lifecycle instructions.
func TestPrintActivationHintLimitsRestartGuidanceToCodex(t *testing.T) {
	withoutCodex := captureStderr(t, func() {
		printActivationHint(install.Result{Agents: []string{"claude"}})
	})
	if withoutCodex != "" {
		t.Fatalf("non-Codex activation hint = %q", withoutCodex)
	}
	withCodex := captureStderr(t, func() {
		printActivationHint(install.Result{Agents: []string{"codex"}})
	})
	if !strings.Contains(withCodex, "Start a new Codex thread") || !strings.Contains(withCodex, "atlas:doctor") {
		t.Fatalf("Codex activation hint = %q", withCodex)
	}
}
