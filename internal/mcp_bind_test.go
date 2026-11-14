package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPBindLabReadOnly(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "session.jsonl")
	record := CommandLog{ID: "test-id", Timestamp: "2026-01-01T00:00:00Z", Command: "sh", Args: []string{"-c", "echo hello"}, Stdout: "hello\n", Duration: "1ms"}
	b, _ := json.Marshal(record)
	if err := os.WriteFile(logPath, append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	markdown := "# Demo\n\n```bash {terminal=true}\necho hello\n```\n"
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "bind_lab", "arguments": map[string]any{"markdown": markdown, "log": logPath}}})
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(string(request)+"\n"), &output, nil); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError || len(result.Result.Content) != 1 || !strings.Contains(result.Result.Content[0].Text, `command-id="test-id"`) {
		t.Fatalf("unexpected response: %s", output.String())
	}
	after, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("binding modified JSONL")
	}
}
