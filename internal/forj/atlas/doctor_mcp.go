package atlas

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const mcpProbeTimeout = 5 * time.Second

// mcpProbeResult describes each boundary proven by the Atlas MCP readiness probe.
type mcpProbeResult struct {
	Executable bool
	Protocol   bool
	Tools      int
	Ready      bool
	Err        error
}

// mcpProbeResponse captures the bounded JSON-RPC response fields used by the readiness probe.
type mcpProbeResponse struct {
	ID     int `json:"id"`
	Result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Tools           []struct {
			Name string `json:"name"`
		} `json:"tools"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// probeMCP verifies the same executable, working directory, protocol, and tool surface configured for local agents.
func probeMCP(parent context.Context, root string) mcpProbeResult {
	command, err := exec.LookPath("forj")
	if err != nil {
		return mcpProbeResult{Err: fmt.Errorf("find forj executable: %w", err)}
	}
	ctx, cancel := context.WithTimeout(parent, mcpProbeTimeout)
	defer cancel()
	result := probeMCPCommand(ctx, command, []string{"atlas:mcp"}, root)
	result.Executable = true
	return result
}

// probeMCPCommand starts one stdio server and proves initialization plus the required Atlas application-info tool.
func probeMCPCommand(ctx context.Context, command string, args []string, root string) mcpProbeResult {
	result := mcpProbeResult{Executable: true}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		result.Err = fmt.Errorf("open Atlas MCP stdin: %w", err)
		return result
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Err = fmt.Errorf("open Atlas MCP stdout: %w", err)
		return result
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		result.Err = fmt.Errorf("start Atlas MCP: %w", err)
		return result
	}
	defer stopMCPProbe(cmd, stdin)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	initialize := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]string{"name": "goforj-atlas-doctor", "version": "1"},
		},
	}
	if err := writeMCPProbeMessage(stdin, initialize); err != nil {
		result.Err = fmt.Errorf("initialize Atlas MCP: %w", err)
		return result
	}
	response, err := readMCPProbeResponse(scanner)
	if err != nil {
		result.Err = mcpProbeFailure("read Atlas MCP initialization", err, stderr.String(), ctx.Err())
		return result
	}
	if response.Error != nil {
		result.Err = fmt.Errorf("initialize Atlas MCP: JSON-RPC %d: %s", response.Error.Code, response.Error.Message)
		return result
	}
	if response.ID != 1 || response.Result.ProtocolVersion == "" {
		result.Err = errors.New("initialize Atlas MCP: response did not negotiate a protocol version")
		return result
	}
	result.Protocol = true

	if err := writeMCPProbeMessage(stdin, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}); err != nil {
		result.Err = fmt.Errorf("confirm Atlas MCP initialization: %w", err)
		return result
	}
	if err := writeMCPProbeMessage(stdin, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}}); err != nil {
		result.Err = fmt.Errorf("list Atlas MCP tools: %w", err)
		return result
	}
	response, err = readMCPProbeResponse(scanner)
	if err != nil {
		result.Err = mcpProbeFailure("read Atlas MCP tools", err, stderr.String(), ctx.Err())
		return result
	}
	if response.Error != nil {
		result.Err = fmt.Errorf("list Atlas MCP tools: JSON-RPC %d: %s", response.Error.Code, response.Error.Message)
		return result
	}
	result.Tools = len(response.Result.Tools)
	for _, tool := range response.Result.Tools {
		if tool.Name == "application-info" {
			result.Ready = true
			return result
		}
	}
	result.Err = errors.New("list Atlas MCP tools: required application-info tool is missing")
	return result
}

// writeMCPProbeMessage sends one newline-delimited JSON-RPC message to the stdio server.
func writeMCPProbeMessage(writer io.Writer, message any) error {
	return json.NewEncoder(writer).Encode(message)
}

// readMCPProbeResponse reads one bounded newline-delimited JSON-RPC response.
func readMCPProbeResponse(scanner *bufio.Scanner) (mcpProbeResponse, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return mcpProbeResponse{}, err
		}
		return mcpProbeResponse{}, io.EOF
	}
	var response mcpProbeResponse
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return mcpProbeResponse{}, fmt.Errorf("decode JSON-RPC response: %w", err)
	}
	return response, nil
}

// mcpProbeFailure preserves the most actionable bounded process or context error.
func mcpProbeFailure(action string, readErr error, stderr string, contextErr error) error {
	if contextErr != nil {
		return fmt.Errorf("%s: %w", action, contextErr)
	}
	if detail := strings.TrimSpace(stderr); detail != "" {
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return fmt.Errorf("%s: %v: %s", action, readErr, detail)
	}
	return fmt.Errorf("%s: %w", action, readErr)
}

// stopMCPProbe closes the request stream and reaps the bounded diagnostic child process.
func stopMCPProbe(cmd *exec.Cmd, stdin io.Closer) {
	_ = stdin.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
