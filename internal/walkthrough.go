package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// WalkthroughItem is either authored narration or a reference to a recorded command.
type WalkthroughItem struct {
	Text   string
	Record *CommandLog
	Pause  time.Duration
	Clear  bool
	Typing time.Duration
}

var (
	boundID        = regexp.MustCompile(`(?:^|\s)command-id="([^"]+)"`)
	boundFile      = regexp.MustCompile(`(?:^|\s)file-id="([^"]+)"`)
	markdownLink   = regexp.MustCompile(`\[([^]]+)\]\([^)]+\)`)
	markdownInline = regexp.MustCompile("[`*_]")
	typingAttr     = regexp.MustCompile(`(?:^|\s)typing=([^\s}]+)`)
	listPrefix     = regexp.MustCompile(`^\s*(?:[-*+]\s+|[0-9]+\.\s+)`)
)

// ParseWalkthrough reads the presentation order from Markdown. Nonterminal code,
// Hugo shortcodes and front matter are not displayed in the terminal cast.
func ParseWalkthrough(data []byte, entries []CommandLog, logPath string) ([]WalkthroughItem, error) {
	if err := ValidateCastRecords(entries); err != nil {
		return nil, err
	}
	byID := make(map[string]CommandLog, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	lines := strings.Split(string(data), "\n")
	items := make([]WalkthroughItem, 0)
	used := make(map[string]bool)
	front := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	inFence := false
	var fenceMarker string
	var opening string
	var script []string
	startLine := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if front {
			if i > 0 && trim == "---" {
				front = false
			}
			continue
		}
		if !inFence && strings.HasPrefix(trim, "```") {
			inFence = true
			fenceMarker = "```"
			opening = trim
			script = nil
			startLine = i + 1
			continue
		}
		if inFence {
			if strings.HasPrefix(trim, fenceMarker) && strings.Trim(trim, "` ") == "" {
				inFence = false
				m := fenceStart.FindStringSubmatch(opening + "\n")
				if m == nil || !terminalAttr.MatchString(m[2]) {
					continue
				}
				id := boundID.FindStringSubmatch(m[2])
				file := boundFile.FindStringSubmatch(m[2])
				if id == nil || file == nil {
					return nil, fmt.Errorf("line %d: terminal fence requires command-id and file-id", startLine)
				}
				if file[1] != filepath.Base(logPath) {
					return nil, fmt.Errorf("line %d: file-id %q does not match log %q", startLine, file[1], filepath.Base(logPath))
				}
				if used[id[1]] {
					return nil, fmt.Errorf("line %d: repeated command-id %q", startLine, id[1])
				}
				e, ok := byID[id[1]]
				if !ok {
					return nil, fmt.Errorf("line %d: command-id %q not found in recording", startLine, id[1])
				}
				if normalizeLabScript(strings.Join(script, "\n")) != normalizeLabScript(recordedScript(e)) {
					return nil, fmt.Errorf("line %d: command text differs from recorded execution %q", startLine, id[1])
				}
				used[id[1]] = true
				copyRecord := e
				var typing time.Duration
				if attr := typingAttr.FindStringSubmatch(m[2]); attr != nil {
					parsed, err := parsePresentationDuration(attr[1], startLine, "typing")
					typing = parsed
					if err != nil {
						return nil, err
					}
				}
				items = append(items, WalkthroughItem{Record: &copyRecord, Typing: typing})
				continue
			}
			script = append(script, line)
			continue
		}
		if match := stepStart.FindStringSubmatch(line); match != nil {
			items = append(items, WalkthroughItem{Text: "\n=== " + match[1] + " ===\n"})
			continue
		}
		if strings.HasPrefix(trim, "<!-- termcast:") {
			if trim == "<!-- termcast:clear -->" {
				items = append(items, WalkthroughItem{Clear: true})
				continue
			}
			if strings.HasPrefix(trim, "<!-- termcast:pause ") && strings.HasSuffix(trim, " -->") {
				durationText := strings.TrimSuffix(strings.TrimPrefix(trim, "<!-- termcast:pause "), " -->")
				duration, err := parsePresentationDuration(durationText, i+1, "pause")
				if err != nil {
					return nil, err
				}
				items = append(items, WalkthroughItem{Pause: duration})
				continue
			}
			return nil, fmt.Errorf("line %d: invalid termcast directive %q", i+1, trim)
		}
		if trim == "" || strings.HasPrefix(trim, "{{<") || strings.HasPrefix(trim, "<!--") || strings.HasPrefix(trim, "-->") {
			continue
		}
		if strings.HasPrefix(trim, "#") {
			trim = strings.TrimSpace(strings.TrimLeft(trim, "#"))
		}
		trim = listPrefix.ReplaceAllString(trim, "")
		trim = markdownLink.ReplaceAllString(trim, "$1")
		trim = markdownInline.ReplaceAllString(trim, "")
		if trim != "" {
			items = append(items, WalkthroughItem{Text: trim})
		}
	}
	if inFence {
		return nil, fmt.Errorf("line %d: unclosed code fence", startLine)
	}
	if len(used) == 0 {
		return nil, fmt.Errorf("no bound terminal commands found")
	}
	return items, nil
}

// Durations are deliberately bounded to keep generated casts watchable.
func parsePresentationDuration(value string, line int, name string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 || d > 30*time.Second {
		return 0, fmt.Errorf("line %d: %s duration %q must be greater than zero and at most 30s", line, name, value)
	}
	return d, nil
}

// WriteWalkthroughCast writes a guided cast with authored narration and recorded
// outputs. It does not execute commands or infer stdout/stderr chunk timing.
func WriteWalkthroughCast(w io.Writer, items []WalkthroughItem, showExitCode ...bool) error {
	bw := bufio.NewWriter(w)
	header := map[string]any{"version": 2, "width": 100, "height": 30, "timestamp": int64(0), "env": map[string]string{"TERM": "xterm-256color"}}
	for _, item := range items {
		if item.Record != nil {
			ts, err := time.Parse(time.RFC3339Nano, item.Record.Timestamp)
			if err != nil {
				return err
			}
			header["timestamp"] = ts.Unix()
			break
		}
	}
	if err := writeCastJSON(bw, header); err != nil {
		return err
	}
	elapsed := 0.0
	emit := func(s string) error {
		err := writeCastJSON(bw, []any{elapsed, "o", normalizeTerminal(s)})
		elapsed += 0.7
		return err
	}
	for _, item := range items {
		if item.Clear {
			if err := writeCastJSON(bw, []any{elapsed, "o", "\x1b[2J\x1b[H"}); err != nil {
				return err
			}
			continue
		}
		if item.Pause > 0 {
			elapsed += item.Pause.Seconds()
			continue
		}
		if item.Record == nil {
			if err := emit("\r\n" + item.Text + "\r\n"); err != nil {
				return err
			}
			continue
		}
		e := item.Record
		if item.Typing > 0 {
			if err := writeCastJSON(bw, []any{elapsed, "o", "\r\n$ "}); err != nil {
				return err
			}
			command := recordedScript(*e)
			for _, r := range command {
				if err := writeCastJSON(bw, []any{elapsed, "o", string(r)}); err != nil {
					return err
				}
				elapsed += item.Typing.Seconds()
			}
			if err := writeCastJSON(bw, []any{elapsed, "o", "\r\n"}); err != nil {
				return err
			}
			elapsed += 0.7
		} else if err := emit("\r\n$ " + recordedScript(*e) + "\r\n"); err != nil {
			return err
		}
		if e.Stdout != "" {
			if err := emit(e.Stdout); err != nil {
				return err
			}
		}
		if e.Stderr != "" {
			if err := emit(e.Stderr); err != nil {
				return err
			}
		}
		if len(showExitCode) > 0 && showExitCode[0] {
			if err := emit(fmt.Sprintf("\r\n[exit %d]\r\n", e.ExitCode)); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

// ExportWalkthroughCast exclusively creates a read-only demo from bound Markdown
// and JSONL, and removes partial output on failure.
func ExportWalkthroughCast(labPath, logPath, output string, showExitCode ...bool) error {
	labAbs, _ := filepath.Abs(labPath)
	logAbs, _ := filepath.Abs(logPath)
	outAbs, _ := filepath.Abs(output)
	if outAbs == labAbs || outAbs == logAbs {
		return fmt.Errorf("output must differ from lab and log")
	}
	data, err := os.ReadFile(labPath)
	if err != nil {
		return err
	}
	entries, err := ReadLogs(logPath)
	if err != nil {
		return err
	}
	items, err := ParseWalkthrough(data, entries, logPath)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := WriteWalkthroughCast(f, items, showExitCode...); err != nil {
		_ = os.Remove(output)
		return err
	}
	return f.Sync()
}
