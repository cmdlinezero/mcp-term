package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LabCommand is a terminal-enabled shell fence in a Markdown lab.
type LabCommand struct {
	Script  string
	Step    string
	Line    int
	Start   int
	End     int
	Opening string
}

var (
	fenceStart   = regexp.MustCompile("^([ \\t]*)```(?:bash|sh)[ \\t]*\\{([^}\\r\\n]*)\\}[ \\t]*\\r?\\n?$")
	fenceEnd     = regexp.MustCompile("^[ \\t]*```[ \\t]*\\r?\\n?$")
	stepStart    = regexp.MustCompile(`\{\{<\s*step\s+[^>]*label="([^"]+)"`)
	terminalAttr = regexp.MustCompile(`(?:^|\s)terminal=true(?:\s|$)`)
	fileAttr     = regexp.MustCompile(`(?:^|\s)file-id="[^"]*"`)
	commandAttr  = regexp.MustCompile(`(?:^|\s)(?:cmd-id|command-id)="[^"]*"`)
)

// normalizeLabScript removes the Markdown list indentation shared by every
// nonblank line. The original Markdown bytes are retained for rewriting.
// Incidental trailing whitespace is ignored when matching recordings.
func normalizeLabScript(script string) string {
	lines := strings.Split(script, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent < 0 {
		return ""
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = strings.TrimRight(line[minIndent:], " \t\r")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ParseLab finds executable fences while leaving other Markdown, including Hugo
// shortcodes, diagrams and hints, untouched. An unclosed terminal fence fails.
func ParseLab(data []byte) ([]LabCommand, error) {
	lines := strings.SplitAfter(string(data), "\n")
	var commands []LabCommand
	offset := 0
	step := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if m := stepStart.FindStringSubmatch(line); m != nil {
			step = m[1]
		}
		match := fenceStart.FindStringSubmatch(line)
		if match == nil || !terminalAttr.MatchString(match[2]) {
			offset += len(line)
			continue
		}
		openingOffset := offset
		scriptStart := offset + len(line)
		j := i + 1
		for ; j < len(lines); j++ {
			if fenceEnd.MatchString(lines[j]) {
				break
			}
		}
		if j == len(lines) {
			return nil, fmt.Errorf("unclosed terminal fence at line %d", i+1)
		}
		script := strings.Join(lines[i+1:j], "")
		script = strings.TrimSuffix(script, "\n")
		script = strings.TrimSuffix(script, "\r")
		script = normalizeLabScript(script)
		if strings.TrimSpace(script) == "" {
			return nil, fmt.Errorf("empty terminal fence at line %d", i+1)
		}
		commands = append(commands, LabCommand{Script: script, Step: step, Line: i + 1, Start: openingOffset, End: scriptStart, Opening: line})
		for k := i; k <= j; k++ {
			offset += len(lines[k])
		}
		i = j
	}
	return commands, nil
}

func recordedScript(e CommandLog) string {
	if (e.Command == "sh" || e.Command == "bash" || e.Command == "/bin/sh") && len(e.Args) == 2 && e.Args[0] == "-c" {
		return e.Args[1]
	}
	return strings.Join(append([]string{e.Command}, e.Args...), " ")
}

// BindLab matches Markdown fences to existing JSONL records in order. It never
// runs commands. A missing or out-of-order record is an error, not a guess.
func BindLab(commands []LabCommand, entries []CommandLog) ([]CommandLog, error) {
	matched := make([]CommandLog, 0, len(commands))
	next := 0
	for _, c := range commands {
		found := -1
		for i := next; i < len(entries); i++ {
			if normalizeLabScript(recordedScript(entries[i])) == normalizeLabScript(c.Script) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("line %d: no ordered recording for %q", c.Line, c.Script)
		}
		if entries[found].ID == "" {
			return nil, fmt.Errorf("line %d: recording has empty execution ID", c.Line)
		}
		matched = append(matched, entries[found])
		next = found + 1
	}
	return matched, nil
}

// ExecuteLab runs each fence as one shell
// invocation through the canonical executor and persists every result to JSONL.
func ExecuteLab(ctx context.Context, commands []LabCommand, logger *Logger) ([]CommandLog, error) {
	out := make([]CommandLog, 0, len(commands))
	for _, c := range commands {
		e := Execute(ctx, "sh", []string{"-c", c.Script}, DefaultTimeout)
		if err := logger.Write(e); err != nil {
			return nil, fmt.Errorf("line %d: persist execution: %w", c.Line, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// RewriteLab changes only terminal fence opening lines, preserving their other
// attributes, content, and surrounding Markdown verbatim.
func RewriteLab(data []byte, commands []LabCommand, matched []CommandLog, logPath string) ([]byte, error) {
	if len(commands) != len(matched) {
		return nil, fmt.Errorf("commands/recordings count mismatch")
	}
	name := filepath.Base(logPath)
	if name == "." || name == "" || strings.ContainsAny(name, "\"\n\r") {
		return nil, fmt.Errorf("invalid log filename")
	}
	var b strings.Builder
	prev := 0
	for i, c := range commands {
		if matched[i].ID == "" || strings.ContainsAny(matched[i].ID, "\"\n\r") {
			return nil, fmt.Errorf("line %d: invalid execution ID", c.Line)
		}
		b.Write(data[prev:c.Start])
		line := c.Opening
		m := fenceStart.FindStringSubmatch(line)
		attrs := m[2]
		attrs = fileAttr.ReplaceAllString(attrs, "")
		attrs = commandAttr.ReplaceAllString(attrs, "")
		attrs = strings.Join(strings.Fields(attrs), " ")
		attrs += fmt.Sprintf(` file-id="%s" command-id="%s"`, name, matched[i].ID)
		prefix := strings.SplitN(line, "{", 2)[0]
		ending := "\n"
		if strings.HasSuffix(line, "\r\n") {
			ending = "\r\n"
		} else if !strings.HasSuffix(line, "\n") {
			ending = ""
		}
		b.WriteString(prefix + "{" + strings.TrimSpace(attrs) + "}" + ending)
		prev = c.End
	}
	b.Write(data[prev:])
	return []byte(b.String()), nil
}

// WriteLabExclusive refuses to overwrite existing content or rewrite its input.
func WriteLabExclusive(input, output string, data []byte) error {
	a, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	b, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if a == b {
		return fmt.Errorf("input and output Markdown paths must differ")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		os.Remove(output)
		return err
	}
	return nil
}
