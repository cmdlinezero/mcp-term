package internal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// castVisibleText reconstructs output across timed asciinema events.
func castVisibleText(t *testing.T, data []byte) string {
	t.Helper()
	var result strings.Builder
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		var event []json.RawMessage
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 3 {
			t.Fatalf("invalid cast event: %s", line)
		}
		var content string
		if err := json.Unmarshal(event[2], &content); err != nil {
			t.Fatal(err)
		}
		result.WriteString(content)
	}
	return result.String()
}

func TestWalkthroughNarrationAndRecordedOutput(t *testing.T) {
	e := sampleCastRecord()
	e.Command = "sh"
	e.Args = []string{"-c", "echo hello"}
	e.Stdout = "recorded hello\n"
	lab := `---
title: Test
---
{{< step label="First step" >}}
Explain the command before running it.
1. Show the result:

` + "```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\"}\necho hello\n```\n" + "```mermaid\ngraph TD\n A-->B\n```\n"
	items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := WriteWalkthroughCast(&b, items); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"First step", "Explain the command", "Show the result", "echo hello", "recorded hello"} {
		if !strings.Contains(castVisibleText(t, b.Bytes()), s) {
			t.Errorf("missing %q", s)
		}
	}
	for _, s := range []string{"graph TD", "title: Test"} {
		if strings.Contains(castVisibleText(t, b.Bytes()), s) {
			t.Errorf("unexpected %q", s)
		}
	}
}
func TestWalkthroughRejectsBrokenBindings(t *testing.T) {
	e := sampleCastRecord()
	e.Command = "sh"
	e.Args = []string{"-c", "echo hello"}
	for _, tc := range []struct{ name, attr, script string }{
		{"missing ID", `file-id="session.jsonl"`, "echo hello"},
		{"wrong file", `file-id="other.jsonl" command-id="abc"`, "echo hello"},
		{"missing record", `file-id="session.jsonl" command-id="unknown"`, "echo hello"},
		{"edited script", `file-id="session.jsonl" command-id="abc"`, "echo changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lab := "```bash {terminal=true " + tc.attr + "}\n" + tc.script + "\n```\n"
			if _, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestWalkthroughExitCodesHiddenByDefault(t *testing.T) {
	e := sampleCastRecord()
	e.ExitCode = 23
	items := []WalkthroughItem{{Text: "Check the environment"}, {Record: &e}}
	for _, show := range []bool{false, true} {
		var b bytes.Buffer
		if err := WriteWalkthroughCast(&b, items, show); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(b.String(), "[exit 23]"); got != show {
			t.Fatalf("show=%v: unexpected exit marker: %s", show, b.String())
		}
		for _, want := range []string{"Check the environment", "go version"} {
			if !strings.Contains(b.String(), want) {
				t.Fatalf("show=%v: missing %q", show, want)
			}
		}
	}
}

func TestWalkthroughTypingWithThemes(t *testing.T) {
	e := sampleCastRecord()
	e.Command = "sh"
	e.Args = []string{"-c", "echo hello"}
	e.Stdout = "recorded hello\n"
	for _, theme := range []string{"", "dracula", "monokai", "github-dark"} {
		t.Run(theme, func(t *testing.T) {
			lab := "```bash {terminal=\"true\" typing=\"35ms\" file-id=\"session.jsonl\" command-id=\"abc\"}\necho hello\n```\n"
			items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			if items[0].TypingDelay.String() != "35ms" {
				t.Fatalf("typing lost: %v", items[0].TypingDelay)
			}
			var b bytes.Buffer
			if err := WriteWalkthroughCastThemed(&b, items, theme); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(b.String(), `"echo hello\\r\\n"`) {
				t.Fatal("command emitted in one event")
			}
			if !strings.Contains(b.String(), `"recorded hello`) {
				t.Fatal("stdout missing")
			}
			if strings.Count(b.String(), `,"o",`) < 10 {
				t.Fatal("not enough timed events")
			}
		})
	}
}

func TestWalkthroughPolishPacingSourceAndDirectory(t *testing.T) {
	cd := sampleCastRecord()
	cd.Command, cd.Args = "sh", []string{"-c", "cd adk-go-tutorial"}
	file := sampleCastRecord()
	file.Command, file.Args = "sh", []string{"-c", "write-file main.go\npackage main\nfunc main() {}"}
	cmd := sampleCastRecord()
	cmd.Command, cmd.Args = "sh", []string{"-c", "go run ."}
	items := []WalkthroughItem{{Record: &cd}, {Record: &file, SourceFile: "main.go"}, {Record: &cmd, TypingDelay: 35 * time.Millisecond}}
	for _, theme := range []string{"", "dracula", "monokai", "github-dark"} {
		var b bytes.Buffer
		if err := WriteWalkthroughCastPolished(&b, items, theme, WalkthroughPresentation{OperationDelay: 200 * time.Millisecond}); err != nil {
			t.Fatal(err)
		}
		text := castVisibleText(t, b.Bytes())
		for _, want := range []string{"── main.go ──", "package", "adk-go-tutorial $ "} {
			if !strings.Contains(text, want) {
				t.Fatalf("theme %q missing %q", theme, want)
			}
		}
		if strings.Contains(text, `"$ write-file`) {
			t.Fatal("source file shown as shell command")
		}
		if strings.Count(b.String(), `,"o",`) < 10 {
			t.Fatal("typing events missing")
		}
	}
	for _, bad := range []time.Duration{-time.Second, 31 * time.Second} {
		if err := WriteWalkthroughCastPolished(&bytes.Buffer{}, items, "", WalkthroughPresentation{OperationDelay: bad}); err == nil {
			t.Fatalf("accepted delay %v", bad)
		}
	}
}

func TestWalkthroughDirectoryInferenceConservative(t *testing.T) {
	for _, script := range []string{"cd", "cd ~", "cd $HOME", "cd ../x", "cd foo && pwd", "cd 'two words'"} {
		if _, ok := nextWalkthroughDir("", script); ok {
			t.Fatalf("inferred ambiguous path: %q", script)
		}
	}
	if got, ok := nextWalkthroughDir("project", "cd src"); !ok || got != "project/src" {
		t.Fatalf("got %q, %v", got, ok)
	}
}

func TestWalkthroughFinalPresentation(t *testing.T) {
	good := sampleCastRecord()
	good.Command, good.Args = "sh", []string{"-c", "echo first\necho second"}
	bad := sampleCastRecord()
	bad.Command, bad.Args, bad.ExitCode = "sh", []string{"-c", "false"}, 7
	source := sampleCastRecord()
	source.Command, source.Args = "sh", []string{"-c", "write-file main.go\npackage main\nfunc main() {}"}
	items := []WalkthroughItem{{Record: &good, TypingDelay: time.Millisecond}, {Record: &source, SourceFile: "main.go"}, {Record: &bad}}
	for _, theme := range []string{"", "dracula", "monokai", "github-dark"} {
		var b bytes.Buffer
		err := WriteWalkthroughCastPolished(&b, items, theme, WalkthroughPresentation{OperationDelay: time.Millisecond, SourceLineDelay: 10 * time.Millisecond, Title: "ADK Lab"})
		if err != nil {
			t.Fatal(err)
		}
		text := castVisibleText(t, b.Bytes())
		for _, want := range []string{"ADK Lab", "── main.go ──", "✓ done", "✗ failed", "── main.go ──"} {
			if !strings.Contains(text, want) {
				t.Fatalf("theme %q missing %q", theme, want)
			}
		}
		if strings.Contains(text, "[write file:") {
			t.Fatal("old source label")
		}
		if strings.Contains(text, "[exit 7]") {
			t.Fatal("numeric exit code visible by default")
		}
		if strings.Count(b.String(), `,"o",`) < 20 {
			t.Fatal("missing progressive events")
		}
	}
	for _, delay := range []time.Duration{-time.Millisecond, 31 * time.Second} {
		if err := WriteWalkthroughCastPolished(&bytes.Buffer{}, items, "", WalkthroughPresentation{SourceLineDelay: delay}); err == nil {
			t.Fatalf("accepted %v", delay)
		}
	}
}

func TestWalkthroughPresentationModesAndCallouts(t *testing.T) {
	e := sampleCastRecord()
	e.Command, e.Args = "sh", []string{"-c", "echo hello"}
	lab := `---
title: Example
---
{{< step label="Setup" >}}
Narrative paragraph.
### 1. Run command
` + "```info\nInfo content\n```\n```hint\nHint content\n```\n```warn\nDo not publish secrets\n```\n```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\"}\necho hello\n```\n" + `### Patterns observed
Additional commentary.
{{< /step >}}
{{< step label="Conclusion" >}}
No executable content.
{{< /step >}}
`
	items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode         string
		want, absent []string
	}{
		{"tutorial", []string{"Setup", "1. Run command", "Narrative paragraph", "INFO", "WARN", "HINT", "Info content", "Hint content", "Do not publish secrets", "Conclusion"}, nil},
		{"demo", []string{"Setup", "1. Run command", "echo", "WARN", "Do not publish secrets"}, []string{"Narrative paragraph", "INFO", "HINT", "Info content", "Hint content", "Patterns observed", "Conclusion"}},
	} {
		for _, theme := range []string{"", "github-dark"} {
			var b bytes.Buffer
			if err := WriteWalkthroughCastPolished(&b, items, theme, WalkthroughPresentation{Mode: tc.mode}); err != nil {
				t.Fatal(err)
			}
			text := castVisibleText(t, b.Bytes())
			for _, v := range tc.want {
				if !strings.Contains(text, v) {
					t.Errorf("%s %s: missing %q", tc.mode, theme, v)
				}
			}
			for _, v := range tc.absent {
				if strings.Contains(text, v) {
					t.Errorf("%s %s: unexpected %q", tc.mode, theme, v)
				}
			}
			if theme == "" && strings.Contains(text, "\\u001b[32mINFO") {
				t.Error("colored callout in plain mode")
			}
		}
	}
	if _, err := selectWalkthroughItems(items, "unknown"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestWalkthroughFramedCalloutsAndSmoothSource(t *testing.T) {
	e := sampleCastRecord()
	e.Command, e.Args = "sh", []string{"-c", "write-file main.go\npackage main\nfunc main() {}"}
	items := []WalkthroughItem{{Kind: "info", Text: "Helpful context"}, {Kind: "warn", Text: "Keep secrets private"}, {Kind: "hint", Text: "Try locally"}, {Record: &e, SourceFile: "main.go"}}
	for _, theme := range []string{"", "github-dark"} {
		var b bytes.Buffer
		if err := WriteWalkthroughCastPolished(&b, items, theme, WalkthroughPresentation{SourceReveal: "smooth"}); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"── INFO ──", "── WARN ──", "── HINT ──", strings.Repeat("─", 40), "Helpful context", "Keep secrets private"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("theme %q missing %q", theme, want)
			}
		}
		if strings.Count(b.String(), `,"o",`) < 12 {
			t.Errorf("theme %q: missing source chunks", theme)
		}
	}
	if err := WriteWalkthroughCastPolished(&bytes.Buffer{}, items, "", WalkthroughPresentation{SourceReveal: "unknown"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
	chunks := smoothSourceChunks("\x1b[31m日本語\x1b[0m\nhello", 2)
	if len(chunks) < 3 || !strings.Contains(strings.Join(chunks, ""), "日本語") {
		t.Fatalf("corrupt chunks: %#v", chunks)
	}
}

func TestWalkthroughTypingPrecedenceAndSpacing(t *testing.T) {
	e := sampleCastRecord()
	e.Command, e.Args = "sh", []string{"-c", "echo hi"}
	for _, tc := range []struct {
		name      string
		attribute string
		override  *time.Duration
		expected  float64
	}{
		{name: "default", expected: .05},
		{name: "markdown", attribute: ` typing="35ms"`, expected: .035},
		{name: "markdown zero", attribute: ` typing="0s"`, expected: 0},
		{name: "cli override", attribute: ` typing="35ms"`, override: durationPtr(10 * time.Millisecond), expected: .01},
		{name: "cli zero", attribute: ` typing="35ms"`, override: durationPtr(0), expected: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lab := "```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\"" + tc.attribute + "}\necho hi\n```\n"
			items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			if err := WriteWalkthroughCastPolished(&b, items, "", WalkthroughPresentation{TypingOverride: tc.override}); err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(b.Bytes()), []byte("\n"))
			var promptTime, firstTime, secondTime float64
			foundPrompt, foundFirst, foundSecond := false, false, false
			for _, line := range lines[1:] {
				var event []json.RawMessage
				if err := json.Unmarshal(line, &event); err != nil {
					t.Fatal(err)
				}
				var tm float64
				var content string
				if err := json.Unmarshal(event[0], &tm); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(event[2], &content); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(content, "\r\n\r\n$ ") {
					foundPrompt = true
					promptTime = tm
				}
				if content == "e" && !foundFirst {
					foundFirst = true
					firstTime = tm
				}
				if content == "c" && !foundSecond {
					foundSecond = true
					secondTime = tm
				}
			}
			if tc.expected == 0 {
				if foundFirst || !strings.Contains(castVisibleText(t, b.Bytes()), "\r\n\r\n$ echo hi") {
					t.Fatal("zero delay must render command immediately after blank line")
				}
			} else {
				if !foundPrompt || !foundFirst || !foundSecond {
					t.Fatal("missing typing events or blank line before prompt")
				}
				if diff := secondTime - firstTime; diff < tc.expected-0.000001 || diff > tc.expected+0.000001 {
					t.Fatalf("typing interval %v, want %v", diff, tc.expected)
				}
				if promptTime > firstTime {
					t.Fatal("prompt after typing")
				}
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }

func TestMultilineContinuationAndCalloutWrap(t *testing.T) {
	e := sampleCastRecord()
	e.Command, e.Args = "sh", []string{"-c", "export A=1\nexport B=2\nexport C=3"}
	lab := "```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\"}\nexport A=1\nexport B=2\nexport C=3\n```\n"
	items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, delay := range []time.Duration{0, 50 * time.Millisecond} {
		var b bytes.Buffer
		if err := WriteWalkthroughCastPolished(&b, items, "", WalkthroughPresentation{TypingOverride: durationPtr(delay)}); err != nil {
			t.Fatal(err)
		}
		visible := castVisibleText(t, b.Bytes())
		for _, expected := range []string{"$ export A=1", "\r\n> export B=2", "\r\n> export C=3"} {
			if !strings.Contains(visible, expected) {
				t.Fatalf("delay %v: missing %q in %q", delay, expected, visible)
			}
		}
	}
	wrapped := wrapCallout(strings.Repeat("longword", 30)+"\nshort text", 100)
	for _, line := range strings.Split(wrapped, "\r\n") {
		if utf8.RuneCountInString(line) > 100 {
			t.Fatalf("unwrapped line: %q", line)
		}
	}
}
