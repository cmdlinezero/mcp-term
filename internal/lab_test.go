package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleLab = `---
title: Demo
---
{{< step label="Check" duration="1:00" >}}

` + "```mermaid\nflowchart TD\n A-->B\n```\n" + "```bash {terminal=true file-id=\"old\" cmd-id=\"old-id\" copy=true}\ngo version\n```\n" + "```bash {terminal=true copy=true}\nprintf 'hi' | cat\n```\n" + "{{< /step >}}\n"

func TestLabParseBindRewrite(t *testing.T) {
	commands, err := ParseLab([]byte(sampleLab))
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || commands[0].Step != "Check" {
		t.Fatalf("commands: %#v", commands)
	}
	entries := []CommandLog{{ID: "abc", Command: "go", Args: []string{"version"}}, {ID: "def", Command: "sh", Args: []string{"-c", "printf 'hi' | cat"}}}
	matched, err := BindLab(commands, entries)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RewriteLab([]byte(sampleLab), commands, matched, "/tmp/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)
	for _, want := range []string{`file-id="session.jsonl" command-id="abc"`, `file-id="session.jsonl" command-id="def"`, `copy=true`, `flowchart TD`, `{{< /step >}}`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `cmd-id=`) || strings.Contains(got, `file-id="old"`) {
		t.Fatal("legacy attributes remain")
	}
}

func TestLabRejectsUnclosedAndUnmatched(t *testing.T) {
	if _, err := ParseLab([]byte("```bash {terminal=true}\necho hi\n")); err == nil {
		t.Fatal("expected unclosed fence error")
	}
	commands, _ := ParseLab([]byte(sampleLab))
	if _, err := BindLab(commands, []CommandLog{{ID: "x", Command: "go", Args: []string{"version"}}}); err == nil {
		t.Fatal("expected missing recording")
	}
}

func TestLabWriteExclusive(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.md")
	output := filepath.Join(dir, "out.md")
	if err := os.WriteFile(input, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteLabExclusive(input, input, []byte("bad")); err == nil {
		t.Fatal("overwrote input")
	}
	if err := WriteLabExclusive(input, output, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := WriteLabExclusive(input, output, []byte("bad")); err == nil {
		t.Fatal("overwrote output")
	}
}

func TestLabExecuteRecordsShellScript(t *testing.T) {
	dir := t.TempDir()
	logger, err := OpenLogger(filepath.Join(dir, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	commands := []LabCommand{{Script: "printf 'hello' | cat", Line: 1}}
	entries, err := ExecuteLab(context.Background(), commands, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Stdout != "hello" || entries[0].ExitCode != 0 {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	saved, err := ReadLogs(filepath.Join(dir, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].ID != entries[0].ID {
		t.Fatal("missing persisted execution ID")
	}
}

func TestLabIndentedDockerFenceMatchesAndPreservesText(t *testing.T) {
	original := "    ```bash {terminal=true file-id=\"docker-tar-1\" cmd-id=\"old\" copy=true}\n    docker version \n    ```\n"
	commands, err := ParseLab([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Script != "docker version" {
		t.Fatalf("unexpected parsed command: %#v", commands)
	}
	matched, err := BindLab(commands, []CommandLog{{ID: "actual-id", Command: "docker", Args: []string{"version"}}})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := RewriteLab([]byte(original), commands, matched, "session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), "    docker version \n") {
		t.Fatal("command text was changed")
	}
	if !strings.Contains(string(rewritten), `command-id="actual-id"`) {
		t.Fatal("missing execution id")
	}
}

func TestLabMultilineIndentNormalization(t *testing.T) {
	input := "    ```bash {terminal=true}\n    printf 'hi' |\n      cat\n    ```\n"
	commands, err := ParseLab([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commands[0].Script, "printf 'hi' |\n  cat"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLabExecuteRecordsFailuresAndContinues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	logger, err := OpenLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	commands := []LabCommand{
		{Script: "printf first", Line: 1},
		{Script: "printf failure >&2; exit 7", Line: 5},
		{Script: "printf last", Line: 9},
	}
	entries, err := ExecuteLab(context.Background(), commands, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Stdout != "first" || entries[1].ExitCode != 7 || entries[1].Stderr != "failure" || entries[2].Stdout != "last" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	saved, err := ReadLogs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 {
		t.Fatalf("expected 3 saved entries, got %d", len(saved))
	}
	for i, e := range saved {
		if e.ID == "" || e.ID != entries[i].ID {
			t.Fatalf("entry %d missing ID", i)
		}
	}
}
