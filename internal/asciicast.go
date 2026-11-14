package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// FindLogByID finds one unambiguous execution record without running commands.
func FindLogByID(entries []CommandLog, id string) (CommandLog, error) {
	if id == "" {
		return CommandLog{}, fmt.Errorf("empty execution ID")
	}
	var found CommandLog
	matches := 0
	for _, e := range entries {
		if e.ID == id {
			found = e
			matches++
		}
	}
	if matches == 0 {
		return CommandLog{}, fmt.Errorf("execution ID %q not found", id)
	}
	if matches != 1 {
		return CommandLog{}, fmt.Errorf("duplicate execution ID %q", id)
	}
	return found, nil
}

// WriteAsciicastV2 converts previously recorded outcomes into a read-only terminal cast.
// Command-level timestamps are available; stdout/stderr chunk timing and interleaving are not.
func WriteAsciicastV2(w io.Writer, entries []CommandLog, showExitCode ...bool) error {
	bw := bufio.NewWriter(w)
	header := map[string]any{"version": 2, "width": 100, "height": 30, "timestamp": int64(0), "env": map[string]string{"TERM": "xterm-256color"}}
	if len(entries) > 0 {
		ts, err := time.Parse(time.RFC3339Nano, entries[0].Timestamp)
		if err != nil {
			return fmt.Errorf("record 1 timestamp: %w", err)
		}
		header["timestamp"] = ts.Unix()
	}
	if err := writeCastJSON(bw, header); err != nil {
		return err
	}
	var previous time.Time
	elapsed := 0.0
	for i, e := range entries {
		if e.ID == "" || strings.TrimSpace(e.Command) == "" {
			return fmt.Errorf("record %d: missing ID or command", i+1)
		}
		ts, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			return fmt.Errorf("record %d timestamp: %w", i+1, err)
		}
		if i > 0 {
			gap := ts.Sub(previous).Seconds()
			if gap < 0 {
				return fmt.Errorf("record %d: timestamps out of order", i+1)
			}
			if gap > 2 {
				gap = 2
			} // Bound idle gaps for watchable playback.
			elapsed += gap
		}
		previous = ts
		prompt := "$ " + e.Command
		for _, arg := range e.Args {
			prompt += " " + shellQuote(arg)
		}
		if err := writeCastJSON(bw, []any{elapsed, "o", prompt + "\r\n"}); err != nil {
			return err
		}
		if e.Stdout != "" {
			if err := writeCastJSON(bw, []any{elapsed + 0.01, "o", normalizeTerminal(e.Stdout)}); err != nil {
				return err
			}
		}
		if e.Stderr != "" {
			if err := writeCastJSON(bw, []any{elapsed + 0.02, "o", normalizeTerminal(e.Stderr)}); err != nil {
				return err
			}
		}
		elapsed += 0.03
		if len(showExitCode) > 0 && showExitCode[0] {
			if err := writeCastJSON(bw, []any{elapsed, "o", fmt.Sprintf("\r\n[exit %d]\r\n", e.ExitCode)}); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

func writeCastJSON(w io.Writer, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
func normalizeTerminal(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\r\n'\"\\$`;&|<>()*?![]{}") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ExportAsciicast reads a JSONL file and creates a new cast file without running commands.
func ExportAsciicast(input, output string, showExitCode ...bool) error {
	if input == output {
		return fmt.Errorf("input and output must differ")
	}
	entries, err := ReadLogs(input)
	if err != nil {
		return err
	}
	if err := ValidateCastRecords(entries); err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := WriteAsciicastV2(f, entries, showExitCode...); err != nil {
		_ = os.Remove(output)
		return err
	}
	return f.Sync()
}
func ValidateCastRecords(entries []CommandLog) error {
	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.ID == "" {
			return fmt.Errorf("record %d: empty execution ID", i+1)
		}
		if seen[e.ID] {
			return fmt.Errorf("record %d: duplicate execution ID %q", i+1, e.ID)
		}
		seen[e.ID] = true
		if _, err := time.Parse(time.RFC3339Nano, e.Timestamp); err != nil {
			return fmt.Errorf("record %d: invalid timestamp: %w", i+1, err)
		}
		if _, err := time.ParseDuration(e.Duration); err != nil {
			return fmt.Errorf("record %d: invalid duration: %w", i+1, err)
		}
	}
	return nil
}
