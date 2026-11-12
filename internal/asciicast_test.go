package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleCastRecord() CommandLog {
	return CommandLog{ID: "abc", Timestamp: "2026-11-12T10:49:11Z", Command: "go", Args: []string{"version"}, Stdout: "go version\n", ExitCode: 0, Duration: "43ms"}
}
func TestFindLogByID(t *testing.T) {
	e := sampleCastRecord()
	if got, err := FindLogByID([]CommandLog{e}, "abc"); err != nil || got.ID != "abc" {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	for _, tc := range []struct {
		name    string
		entries []CommandLog
		id      string
	}{{"missing", []CommandLog{e}, "x"}, {"duplicate", []CommandLog{e, e}, "abc"}, {"empty", []CommandLog{e}, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FindLogByID(tc.entries, tc.id); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
func TestAsciicastV2(t *testing.T) {
	e := sampleCastRecord()
	e.Stderr = "warning\n"
	e.ExitCode = 42
	var b bytes.Buffer
	if err := WriteAsciicastV2(&b, []CommandLog{e}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected header and four events, got %d: %s", len(lines), b.String())
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil || header["version"] != float64(2) {
		t.Fatalf("bad header %v %v", header, err)
	}
	for i, line := range lines[1:] {
		var event []any
		if err := json.Unmarshal([]byte(line), &event); err != nil || len(event) != 3 || event[1] != "o" {
			t.Fatalf("event %d: %s %v", i, line, err)
		}
	}
	for _, text := range []string{"$ go version", "go version", "warning", "exit 42"} {
		if !strings.Contains(b.String(), text) {
			t.Errorf("missing %q", text)
		}
	}
}
func TestCastValidation(t *testing.T) {
	e := sampleCastRecord()
	for _, tc := range []struct {
		name    string
		entries []CommandLog
	}{{"valid", []CommandLog{e}}, {"duplicate", []CommandLog{e, e}}, {"no-id", []CommandLog{{Command: "go", Timestamp: e.Timestamp, Duration: e.Duration}}}, {"bad-time", []CommandLog{{ID: "a", Command: "go", Timestamp: "invalid", Duration: e.Duration}}}, {"bad-duration", []CommandLog{{ID: "a", Command: "go", Timestamp: e.Timestamp, Duration: "invalid"}}}} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCastRecords(tc.entries)
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}
func TestExportReadOnlyAndSafety(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "session.cast")
	logger, err := OpenLogger(input)
	if err != nil {
		t.Fatal(err)
	}
	e := sampleCastRecord()
	e.Command = "command-that-must-never-be-run"
	if err := logger.Write(e); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ExportAsciicast(input, output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if err := ExportAsciicast(input, output); err == nil {
		t.Fatal("must not overwrite output")
	}
	if err := ExportAsciicast(input, input); err == nil {
		t.Fatal("must reject same path")
	}
}
func TestCorruptJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLogs(path); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected line-numbered error: %v", err)
	}
}
