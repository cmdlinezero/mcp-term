package internal

import (
	"bytes"
	"strings"
	"testing"
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

func TestWalkthroughBlockTypingAndOneOffControls(t *testing.T) {
	e := sampleCastRecord()
	e.Command = "sh"
	e.Args = []string{"-c", "echo hello"}
	e.Stdout = "result\n"
	lab := "# Demo\n<!-- termcast:pause 2s -->\n<!-- termcast:clear -->\n```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\" typing=35ms}\necho hello\n```\n"
	items, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[1].Pause.Seconds() != 2 || !items[2].Clear || items[3].Typing.Milliseconds() != 35 {
		t.Fatalf("unexpected parsed items: %+v", items)
	}
	var b bytes.Buffer
	if err := WriteWalkthroughCast(&b, items); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `\u001b[2J\u001b[H`) || !strings.Contains(b.String(), "result") {
		t.Fatalf("missing clear or output: %s", b.String())
	}
	if strings.Contains(b.String(), `"$ echo hello`) {
		t.Fatal("command was not typed")
	}
}

func TestWalkthroughRejectsBadPresentation(t *testing.T) {
	e := sampleCastRecord()
	e.Command = "sh"
	e.Args = []string{"-c", "echo hello"}
	for _, line := range []string{"<!-- termcast:pause 0s -->", "<!-- termcast:pause nonsense -->", "<!-- termcast:typing 30ms -->", "<!-- termcast:pause 31s -->"} {
		lab := line + "\n```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\"}\necho hello\n```\n"
		if _, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl"); err == nil {
			t.Fatalf("accepted %s", line)
		}
	}
	lab := "```bash {terminal=true file-id=\"session.jsonl\" command-id=\"abc\" typing=0ms}\necho hello\n```\n"
	if _, err := ParseWalkthrough([]byte(lab), []CommandLog{e}, "session.jsonl"); err == nil {
		t.Fatal("accepted invalid typing")
	}
}

func TestRewriteLabPreservesTypingAttribute(t *testing.T) {
	source := []byte("```bash {terminal=true typing=35ms}\necho hello\n```\n")
	commands, err := ParseLab(source)
	if err != nil {
		t.Fatal(err)
	}
	e := sampleCastRecord()
	result, err := RewriteLab(source, commands, []CommandLog{e}, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "typing=35ms") {
		t.Fatalf("typing attribute lost: %s", result)
	}
}
