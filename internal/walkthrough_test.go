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
