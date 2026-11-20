package internal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

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
		if !strings.Contains(b.String(), s) {
			t.Errorf("missing %q", s)
		}
	}
	for _, s := range []string{"graph TD", "title: Test"} {
		if strings.Contains(b.String(), s) {
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
		text := b.String()
		for _, want := range []string{"── main.go ──", "package", "adk-go-tutorial $ "} {
			if !strings.Contains(text, want) {
				t.Fatalf("theme %q missing %q", theme, want)
			}
		}
		if strings.Contains(text, `"$ write-file`) {
			t.Fatal("source file shown as shell command")
		}
		if strings.Count(text, `,"o",`) < 10 {
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
		text := b.String()
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
		if strings.Count(text, `,"o",`) < 20 {
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
			text := b.String()
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
