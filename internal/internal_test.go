package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteAndIDs(t *testing.T) {
	a := Execute(context.Background(), "go", []string{"version"}, time.Second*10)
	b := Execute(context.Background(), "go", []string{"version"}, time.Second*10)
	if a.ExitCode != 0 || b.ExitCode != 0 {
		t.Fatalf("execution failed: %+v %+v", a, b)
	}
	if len(a.ID) != 16 || a.ID == b.ID {
		t.Fatalf("invalid or duplicate IDs: %s %s", a.ID, b.ID)
	}
}
func TestFailure(t *testing.T) {
	e := Execute(context.Background(), "this-command-does-not-exist-123", nil, time.Second)
	if e.ExitCode != -1 || e.Stderr == "" {
		t.Fatalf("unexpected failure: %+v", e)
	}
}
func TestLogRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, err := OpenLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	e := Execute(context.Background(), "go", []string{"version"}, time.Second*10)
	if err := l.Write(e); err != nil {
		t.Fatal(err)
	}
	l.Close()
	entries, err := ReadLogs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != e.ID {
		t.Fatalf("bad round trip: %+v", entries)
	}
}
func TestMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, err := OpenLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run_command","arguments":{"command":"go","args":["version"]}}}`,
		`not-json`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &output, l); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected four responses, got %d: %s", len(lines), output.String())
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("tool call failed: %v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
