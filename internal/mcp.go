package internal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const protocolVersion = "2024-11-05"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type runParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type runArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, logger *Logger) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	writer := bufio.NewWriter(out)
	send := func(r response) error {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err = writer.Write(append(b, '\n')); err != nil {
			return err
		}
		return writer.Flush()
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "Parse error"}}); err != nil {
				return err
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			if err := send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32600, "Invalid Request"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 { // Notifications never receive responses.
			continue
		}
		r := response{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			r.Result = map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "mcp-term", "version": "0.1.0"}}
		case "ping":
			r.Result = map[string]any{}
		case "tools/list":
			r.Result = map[string]any{"tools": []any{map[string]any{"name": "run_command", "description": "Execute a local program directly, record stdout, stderr, exit code and duration. Commands are NOT sandboxed.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "Executable name or path"}, "args": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"command"}, "additionalProperties": false}}}}
		case "tools/call":
			var p runParams
			if err := json.Unmarshal(req.Params, &p); err != nil || p.Name != "run_command" {
				r.Error = &rpcError{-32602, "Invalid tool name or parameters"}
				break
			}
			var a runArgs
			if err := json.Unmarshal(p.Arguments, &a); err != nil || strings.TrimSpace(a.Command) == "" {
				r.Error = &rpcError{-32602, "command is required and must be a string"}
				break
			}
			entry := Execute(ctx, a.Command, a.Args, DefaultTimeout)
			if err := logger.Write(entry); err != nil {
				return fmt.Errorf("persist command log: %w", err)
			}
			data, _ := json.Marshal(entry)
			r.Result = map[string]any{"content": []any{map[string]string{"type": "text", "text": string(data)}}, "isError": entry.ExitCode != 0}
		default:
			r.Error = &rpcError{-32601, "Method not found"}
		}
		if err := send(r); err != nil {
			return err
		}
	}
	return scanner.Err()
}
