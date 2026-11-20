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
	SourceFile  string
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
				sourceFile := ""
				if m[2] != "bash" && m[2] != "sh" {
					sourceFile = destAttr.FindStringSubmatch(fenceAttrs(m))[1]
				}
				items = append(items, WalkthroughItem{Record: &copyRecord, TypingDelay: delay, SourceFile: sourceFile})
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

// WalkthroughPresentation controls only generated cast events, never recordings.
type WalkthroughPresentation struct {
	OperationDelay  time.Duration
	SourceLineDelay time.Duration // Optional line-by-line source reveal; zero displays immediately.
	Title           string        // Optional terminal header title.
}

func WriteWalkthroughCastThemed(w io.Writer, items []WalkthroughItem, theme string, showExitCode ...bool) error {
	return WriteWalkthroughCastPolished(w, items, theme, WalkthroughPresentation{OperationDelay: 700 * time.Millisecond}, showExitCode...)
}

// WriteWalkthroughCastPolished renders bound Markdown with explicit inter-operation pacing.
func WriteWalkthroughCastPolished(w io.Writer, items []WalkthroughItem, theme string, presentation WalkthroughPresentation, showExitCode ...bool) error {
	if presentation.OperationDelay < 0 || presentation.OperationDelay > 30*time.Second {
		return fmt.Errorf("operation delay must be between 0 and 30s")
	}
	if presentation.SourceLineDelay < 0 || presentation.SourceLineDelay > 30*time.Second {
		return fmt.Errorf("source line delay must be between 0 and 30s")
	}
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
		return writeCastJSON(bw, []any{elapsed, "o", normalizeTerminal(s)})
	}
	// The header uses printable ANSI styling only, without cursor positioning.
	title := strings.TrimSpace(presentation.Title)
	if title == "" {
		title = "TermCast walkthrough"
	}
	if strings.ContainsAny(title, "\r\n\x1b") {
		return fmt.Errorf("walkthrough title must be a single plain-text line")
	}
	if err := emit("\x1b[2m┌─ " + title + " ─┐\x1b[0m\r\n"); err != nil {
		return err
	}
	// Directory context is inferred only from successful, simple standalone cd commands.
	// No initial directory is invented: the prompt remains "$ " until known.
	cwd := ""
	advance := func() { elapsed += presentation.OperationDelay.Seconds() }
	for _, item := range items {
		if item.Record == nil {
			if err := emit("\r\n" + item.Text + "\r\n"); err != nil {
				return err
			}
			advance()
			continue
		}
		e := item.Record
		script := recordedScript(*e)
		if item.SourceFile != "" {
			// Source-file operations have a visible destination, not a misleading shell prompt.
			if err := emit("\r\n\x1b[2m── " + item.SourceFile + " ──\x1b[0m\r\n"); err != nil {
				return err
			}
			script = strings.TrimPrefix(script, "write-file "+item.SourceFile+"\n")
		}
		highlighted := HighlightCastInput(script, theme)
		// Keep the existing Go syntax mode for file bodies, without displaying the
		// internal write-file recording representation.
		if item.SourceFile != "" && theme != "" {
			colored := HighlightCastInput(recordedScript(*e), theme)
			if pos := strings.IndexByte(colored, '\n'); pos >= 0 {
				highlighted = colored[pos+1:]
			}
		}
		prompt := "$ "
		if cwd != "" {
			prompt = cwd + " $ "
		}
		if item.SourceFile != "" {
			prompt = ""
		}
		if item.SourceFile != "" && presentation.SourceLineDelay > 0 {
			if err := emit("\r\n"); err != nil {
				return err
			}
			// Split only on actual newlines; ANSI color sequences remain intact.
			for _, line := range strings.SplitAfter(highlighted, "\n") {
				if line == "" {
					continue
				}
				if err := emit(line); err != nil {
					return err
				}
				elapsed += presentation.SourceLineDelay.Seconds()
			}
			if !strings.HasSuffix(highlighted, "\n") {
				if err := emit("\r\n"); err != nil {
					return err
				}
			}
		} else if item.TypingDelay <= 0 {
			if err := emit("\r\n" + prompt + highlighted + "\r\n"); err != nil {
				return err
			}
		} else {
			if err := emit("\r\n" + prompt); err != nil {
				return err
			}
			// ANSI SGR sequences are instantaneous; visible runes consume typing time.
			// Newlines are normalized as terminal line breaks, not literal LF.
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
				r, width := utf8.DecodeRuneInString(highlighted)
				chunk := highlighted[:width]
				if r == '\n' {
					chunk = "\r\n"
				}
				if err := writeCastJSON(bw, []any{elapsed, "o", chunk}); err != nil {
					return err
				}
				elapsed += item.TypingDelay.Seconds()
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
		} else if e.ExitCode == 0 {
			if err := emit("\r\n\x1b[2m✓ done\x1b[0m\r\n"); err != nil {
				return err
			}
		} else {
			if err := emit("\r\n\x1b[31m✗ failed\x1b[0m\r\n"); err != nil {
				return err
			}
		}
		if e.ExitCode == 0 && item.SourceFile == "" {
			if next, ok := nextWalkthroughDir(cwd, script); ok {
				cwd = next
			}
		}
		advance()
	}
	return bw.Flush()
}

// ExportWalkthroughCast exclusively creates a read-only demo from bound Markdown
// and JSONL, and removes partial output on failure.
func ExportWalkthroughCast(labPath, logPath, output string, showExitCode ...bool) error {
	return ExportWalkthroughCastThemed(labPath, logPath, output, "", showExitCode...)
}

func ExportWalkthroughCastThemed(labPath, logPath, output, theme string, showExitCode ...bool) error {
	return ExportWalkthroughCastPolished(labPath, logPath, output, theme, WalkthroughPresentation{OperationDelay: 700 * time.Millisecond}, showExitCode...)
}

func ExportWalkthroughCastPolished(labPath, logPath, output, theme string, presentation WalkthroughPresentation, showExitCode ...bool) error {
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
	// Prefer the authored front-matter title when the caller has not supplied one.
	if presentation.Title == "" {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "title:") {
				presentation.Title = strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "title:")), "\"'")
				break
			}
		}
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
	if err := WriteWalkthroughCastPolished(f, items, theme, presentation, showExitCode...); err != nil {
		_ = os.Remove(output)
		return err
	}
	return f.Sync()
}

// nextWalkthroughDir intentionally handles only unambiguous standalone cd paths.
// Compound shell commands, variables, and home-directory expansion are not guessed.
func nextWalkthroughDir(current, script string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(script))
	if len(fields) != 2 || fields[0] != "cd" {
		return "", false
	}
	path := fields[1]
	if strings.ContainsAny(path, "\"'`$;|&\\") || strings.HasPrefix(path, "~") || path == "-" {
		return "", false
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}
	if current == "" && (path == ".." || strings.HasPrefix(path, "../")) {
		return "", false
	}
	return filepath.Clean(filepath.Join(current, path)), true
}
