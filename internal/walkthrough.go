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
	"unicode/utf8"
)

// WalkthroughItem is either authored narration or a reference to a recorded command.
type WalkthroughItem struct {
	Text        string
	Record      *CommandLog
	TypingDelay time.Duration
}

var (
	boundID         = regexp.MustCompile(`(?:^|\s)command-id="([^"]+)"`)
	typingAttribute = regexp.MustCompile(`(?:^|\s)typing="?([0-9]+(?:ms|s))"?`)
	boundFile       = regexp.MustCompile(`(?:^|\s)file-id="([^"]+)"`)
	markdownLink    = regexp.MustCompile(`\[([^]]+)\]\([^)]+\)`)
	markdownInline  = regexp.MustCompile("[`*_]")
	listPrefix      = regexp.MustCompile(`^\s*(?:[-*+]\s+|[0-9]+\.\s+)`)
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
				if m == nil || !terminalAttr.MatchString(fenceAttrs(m)) {
					continue
				}
				id := boundID.FindStringSubmatch(fenceAttrs(m))
				file := boundFile.FindStringSubmatch(fenceAttrs(m))
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
				expected := strings.Join(script, "\n")
				if m[2] != "bash" && m[2] != "sh" {
					dest := destAttr.FindStringSubmatch(fenceAttrs(m))
					if dest == nil {
						return nil, fmt.Errorf("line %d: source block requires file=", startLine)
					}
					expected = sourceScript(LabCommand{Script: normalizeLabScript(expected), File: dest[1]})
				}
				if normalizeLabScript(expected) != normalizeLabScript(recordedScript(e)) {
					return nil, fmt.Errorf("line %d: command text differs from recorded execution %q", startLine, id[1])
				}
				used[id[1]] = true
				copyRecord := e
				var delay time.Duration
				if attr := typingAttribute.FindStringSubmatch(fenceAttrs(m)); attr != nil {
					var err error
					delay, err = time.ParseDuration(attr[1])
					if err != nil || delay <= 0 {
						return nil, fmt.Errorf("line %d: invalid typing duration %q", startLine, attr[1])
					}
				}
				items = append(items, WalkthroughItem{Record: &copyRecord, TypingDelay: delay})
				continue
			}
			script = append(script, line)
			continue
		}
		if match := stepStart.FindStringSubmatch(line); match != nil {
			items = append(items, WalkthroughItem{Text: "\n=== " + match[1] + " ===\n"})
			continue
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

// WriteWalkthroughCast writes a guided cast with authored narration and recorded
// outputs. It does not execute commands or infer stdout/stderr chunk timing.
func WriteWalkthroughCast(w io.Writer, items []WalkthroughItem, showExitCode ...bool) error {
	return WriteWalkthroughCastThemed(w, items, "", showExitCode...)
}

func WriteWalkthroughCastThemed(w io.Writer, items []WalkthroughItem, theme string, showExitCode ...bool) error {
	if err := ValidateHighlightTheme(theme); err != nil {
		return err
	}
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
		if item.Record == nil {
			if err := emit("\r\n" + item.Text + "\r\n"); err != nil {
				return err
			}
			continue
		}
		e := item.Record
		highlighted := HighlightCastInput(recordedScript(*e), theme)
		if item.TypingDelay <= 0 {
			if err := emit("\r\n$ " + highlighted + "\r\n"); err != nil {
				return err
			}
		} else {
			if err := emit("\r\n$ "); err != nil {
				return err
			}
			// ANSI escape sequences do not consume typing time; each visible
			// Unicode character gets its own output event.
			delay := item.TypingDelay.Seconds()
			for len(highlighted) > 0 {
				if highlighted[0] == '\x1b' {
					end := strings.IndexByte(highlighted, 'm')
					if end < 0 {
						return fmt.Errorf("unterminated highlight escape")
					}
					if err := writeCastJSON(bw, []any{elapsed, "o", highlighted[:end+1]}); err != nil {
						return err
					}
					highlighted = highlighted[end+1:]
					continue
				}
				_, width := utf8.DecodeRuneInString(highlighted)
				if err := writeCastJSON(bw, []any{elapsed, "o", normalizeTerminal(highlighted[:width])}); err != nil {
					return err
				}
				elapsed += delay
				highlighted = highlighted[width:]
			}
			if err := emit("\r\n"); err != nil {
				return err
			}
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
	return ExportWalkthroughCastThemed(labPath, logPath, output, "", showExitCode...)
}

func ExportWalkthroughCastThemed(labPath, logPath, output, theme string, showExitCode ...bool) error {
	if err := ValidateHighlightTheme(theme); err != nil {
		return err
	}
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
	if err := WriteWalkthroughCastThemed(f, items, theme, showExitCode...); err != nil {
		_ = os.Remove(output)
		return err
	}
	return f.Sync()
}