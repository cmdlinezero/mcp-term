package internal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LabCommand is a terminal-enabled shell fence in a Markdown lab.
type LabCommand struct {
	Script    string
	Step      string
	Line      int
	Start     int
	End       int
	Opening   string
	Language  string
	File      string
	Overwrite bool
}

var (
	fenceStart    = regexp.MustCompile("^([ \t]*)```(bash|sh|[A-Za-z][A-Za-z0-9_+.-]*)[ \t]*(?:\\{([^}\\r\\n]*)\\}|([^\\r\\n]*))[ \t]*\\r?\\n?$")
	fenceEnd      = regexp.MustCompile("^[ \t]*```[ \t]*\\r?\\n?$")
	stepStart     = regexp.MustCompile(`\{\{<\s*step\s+[^>]*label="([^"]+)"`)
	terminalAttr  = regexp.MustCompile(`(?:^|\s)terminal=(?:"true"|true)(?:\s|$)`)
	fileAttr      = regexp.MustCompile(`(?:^|\s)file-id=(?:"[^"]*"|[^\s]+)`)
	commandAttr   = regexp.MustCompile(`(?:^|\s)(?:cmd-id|command-id)=(?:"[^"]*"|[^\s]+)`)
	destAttr      = regexp.MustCompile(`(?:^|\s)file="([^"]+)"(?:\s|$)`)
	overwriteAttr = regexp.MustCompile(`(?:^|\s)overwrite=(?:"true"|true)(?:\s|$)`)
)

func fenceAttrs(m []string) string {
	if len(m) < 5 {
		return ""
	}
	if m[3] != "" {
		return m[3]
	}
	return m[4]
}

// sourceScript is a stable JSONL representation of a source-file write.
// It is never executed as a shell command.
func sourceScript(c LabCommand) string { return "write-file " + c.File + "\n" + c.Script }

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
		if match == nil || !terminalAttr.MatchString(fenceAttrs(match)) {
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
		language := match[2]
		attrs := fenceAttrs(match)
		c := LabCommand{Script: script, Step: step, Line: i + 1, Start: openingOffset, End: scriptStart, Opening: line, Language: language}
		if language != "bash" && language != "sh" {
			m := destAttr.FindStringSubmatch(attrs)
			if m == nil {
				return nil, fmt.Errorf("line %d: source fence %q requires file= destination", i+1, language)
			}
			c.File = m[1]
			c.Overwrite = overwriteAttr.MatchString(attrs)
		}
		commands = append(commands, c)
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
			expected := c.Script
			if c.File != "" {
				expected = sourceScript(c)
			}
			if normalizeLabScript(recordedScript(entries[i])) == normalizeLabScript(expected) {
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

// ExecuteLab runs shell commands and materializes source files within a lab
// workspace. A standalone cd command changes the working directory of later
// blocks. All operations produce JSONL records, including failures.
func ExecuteLab(ctx context.Context, commands []LabCommand, logger *Logger) ([]CommandLog, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cwd := root
	out := make([]CommandLog, 0, len(commands))
	for _, c := range commands {
		var e CommandLog
		if c.File != "" {
			e = writeSource(c, root, cwd)
		} else if next, ok := labCD(c.Script); ok {
			e = labChangeDir(c, root, cwd, next)
			if e.ExitCode == 0 {
				cwd = e.Stdout
				e.Stdout = ""
			}
		} else {
			e = executeLabShell(ctx, c.Script, cwd)
		}
		if err := logger.Write(e); err != nil {
			return nil, fmt.Errorf("line %d: persist execution: %w", c.Line, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func labCD(script string) (string, bool) {
	fields := strings.Fields(script)
	if len(fields) == 2 && fields[0] == "cd" {
		return fields[1], true
	}
	return "", false
}

func newLabRecord(command string, args []string, start time.Time, exit int, stdout, stderr string) CommandLog {
	e := CommandLog{Timestamp: start.UTC().Format(time.RFC3339Nano), Command: command, Args: args, ExitCode: exit, Stdout: stdout, Stderr: stderr, Duration: time.Since(start).String()}
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	payload, _ := json.Marshal([]any{e.Command, e.Args, e.Timestamp, e.ExitCode, e.Stdout, hex.EncodeToString(nonce)})
	hash := sha256.Sum256(payload)
	e.ID = hex.EncodeToString(hash[:8])
	return e
}

func labChangeDir(c LabCommand, root, cwd, next string) CommandLog {
	start := time.Now()
	dest := filepath.Join(cwd, next)
	resolved, err := filepath.EvalSymlinks(dest)
	if err == nil {
		var info os.FileInfo
		info, err = os.Stat(resolved)
		if err == nil && !info.IsDir() {
			err = fmt.Errorf("not a directory: %s", next)
		}
	}
	if err == nil {
		rel, relErr := filepath.Rel(root, resolved)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			err = fmt.Errorf("directory escapes lab workspace: %s", next)
		}
	}
	if err != nil {
		return newLabRecord("sh", []string{"-c", c.Script}, start, 1, "", err.Error())
	}
	return newLabRecord("sh", []string{"-c", c.Script}, start, 0, resolved, "")
}

func writeSource(c LabCommand, root, cwd string) CommandLog {
	start := time.Now()
	args := []string{"-c", sourceScript(c)}
	fail := func(err error) CommandLog { return newLabRecord("sh", args, start, 1, "", err.Error()) }
	if c.File == "" || filepath.IsAbs(c.File) {
		return fail(fmt.Errorf("invalid destination %q", c.File))
	}
	dest := filepath.Join(cwd, c.File)
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail(fmt.Errorf("destination escapes lab workspace: %s", c.File))
	}
	// Prevent symlink traversal through any existing parent component.
	parent := filepath.Dir(dest)
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fail(err)
	}
	rel, err = filepath.Rel(root, realParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail(fmt.Errorf("destination parent escapes workspace: %s", c.File))
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if c.Overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	// Reject symlinks even with overwrite=true.
	if info, statErr := os.Lstat(dest); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return fail(fmt.Errorf("destination is a symlink: %s", c.File))
	}
	f, err := os.OpenFile(dest, flags, 0600)
	if err != nil {
		return fail(err)
	}
	_, writeErr := f.WriteString(c.Script + "\n")
	closeErr := f.Close()
	if writeErr != nil {
		return fail(writeErr)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	return newLabRecord("sh", args, start, 0, "", "")
}

func executeLabShell(ctx context.Context, script, cwd string) CommandLog {
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", script)
	cmd.Dir = cwd
	var stdout, stderr limitedBuffer
	stdout.limit = MaxOutputBytes
	stderr.limit = MaxOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := 0
	if err != nil {
		exit = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else if stderr.Len() == 0 {
			_, _ = stderr.Write([]byte(err.Error()))
		}
	}
	if cctx.Err() != nil {
		exit = -1
		_, _ = stderr.Write([]byte("\nexecution interrupted: " + cctx.Err().Error()))
	}
	return newLabRecord("sh", []string{"-c", script}, start, exit, stdout.String(), stderr.String())
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
		attrs := fenceAttrs(m)
		attrs = fileAttr.ReplaceAllString(attrs, "")
		attrs = commandAttr.ReplaceAllString(attrs, "")
		attrs = strings.Join(strings.Fields(attrs), " ")
		attrs += fmt.Sprintf(` file-id="%s" command-id="%s"`, name, matched[i].ID)
		prefix := m[1] + "```" + m[2] + " "
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
