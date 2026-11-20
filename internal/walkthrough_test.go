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
		for _, want := range []string{"[write file: main.go]", "package", "adk-go-tutorial $ "} {
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
