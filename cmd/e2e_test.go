package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/example/mcp-term/internal"
)

const tutorialFixture = `---
title: Regression Lab
---
{{< step label="Run" >}}
    ` + "```" + `bash {terminal=true file-id="legacy" cmd-id="old-1" copy=true}
    printf 'hello\n'
    ` + "```" + `
    ` + "```" + `mermaid
    graph LR
      A --> B
    ` + "```" + `
    ` + "```" + `bash {terminal=true file-id="legacy" cmd-id="old-2" copy=true}
    printf 'world\n' | tr a-z A-Z
    ` + "```" + `
{{< /step >}}
`

func runCLI(args ...string) error {
	c := newRoot()
	c.SetArgs(args)
	c.SetOut(os.NewFile(1, "stdout"))
	return c.Execute()
}

func TestE2EMarkdownRecordBindExport(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("requires POSIX sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "lab.md")
	log := filepath.Join(dir, "session.jsonl")
	bound := filepath.Join(dir, "bound.md")
	cast := filepath.Join(dir, "session.cast")
	if err := os.WriteFile(input, []byte(tutorialFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCLI("lab-import", "--input", input, "--log", log, "--output", bound); err != nil {
		t.Fatal(err)
	}
	entries, err := internal.ReadLogs(log)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("record count = %d, want 2", len(entries))
	}
	ids := map[string]bool{}
	for i, e := range entries {
		if e.ID == "" || ids[e.ID] {
			t.Fatalf("missing/duplicate ID at %d", i)
		}
		ids[e.ID] = true
		if e.Command != "sh" || len(e.Args) != 2 || e.Args[0] != "-c" || e.ExitCode != 0 || e.Timestamp == "" || e.Duration == "" {
			t.Fatalf("invalid record %d: %+v", i, e)
		}
		if _, err := json.Marshal(e); err != nil {
			t.Fatal(err)
		}
	}
	if entries[0].Stdout != "hello\n" || entries[1].Stdout != "WORLD\n" {
		t.Fatalf("stdout: %q, %q", entries[0].Stdout, entries[1].Stdout)
	}
	original, _ := os.ReadFile(input)
	if string(original) != tutorialFixture {
		t.Fatal("input Markdown modified")
	}
	generated, err := os.ReadFile(bound)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "graph LR") || !strings.Contains(string(generated), `{{< step label="Run" >}}`) {
		t.Fatal("nonterminal content lost")
	}
	for _, e := range entries {
		if !strings.Contains(string(generated), `command-id="`+e.ID+`"`) {
			t.Fatalf("missing bound ID %s", e.ID)
		}
	}
	if strings.Contains(string(generated), "cmd-id=") || strings.Count(string(generated), `file-id="session.jsonl"`) != 2 {
		t.Fatal("incorrect metadata rewrite")
	}
	commands, err := internal.ParseLab(generated)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := internal.BindLab(commands, entries)
	if err != nil || len(matched) != 2 {
		t.Fatalf("binding: %v", err)
	}
	if err := runCLI("export-asciinema", "--log", log, "--output", cast); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cast)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 5 {
		t.Fatalf("cast too short: %d lines", len(lines))
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header["version"] != float64(2) {
		t.Fatalf("cast version: %v", header["version"])
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "WORLD") {
		t.Fatal("missing recorded output in cast")
	}
	if err := runCLI("export-asciinema", "--log", log, "--output", cast); err == nil {
		t.Fatal("cast overwrite accepted")
	}
	if err := runCLI("lab-import", "--input", input, "--log", log); err == nil {
		t.Fatal("log overwrite accepted")
	}
	if err := runCLI("lab-import", "--input", input, "--log", filepath.Join(dir, "other.jsonl"), "--output", bound); err == nil {
		t.Fatal("Markdown overwrite accepted")
	}
	rebound := filepath.Join(dir, "rebound.md")
	if err := runCLI("lab-import", "--input", input, "--log", log, "--output", rebound, "--bind-only"); err != nil {
		t.Fatal(err)
	}
	reboundData, _ := os.ReadFile(rebound)
	if string(reboundData) != string(generated) {
		t.Fatal("bind-only differs from execution binding")
	}
}

func TestE2EFailureRecordedAndFollowingCommandRuns(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("requires POSIX sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "lab.md")
	log := filepath.Join(dir, "session.jsonl")
	bound := filepath.Join(dir, "bound.md")
	md := "```bash {terminal=true}\nprintf 'bad\\n' >&2; exit 7\n```\n\n```sh {terminal=true}\nprintf 'after\\n'\n```\n"
	if err := os.WriteFile(input, []byte(md), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCLI("lab-import", "--input", input, "--log", log, "--output", bound); err == nil || !regexp.MustCompile(`1 of 2 commands exited nonzero`).MatchString(err.Error()) {
		t.Fatalf("expected summary error, got %v", err)
	}
	entries, err := internal.ReadLogs(log)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ExitCode != 7 || !strings.Contains(entries[0].Stderr, "bad") || entries[1].ExitCode != 0 || entries[1].Stdout != "after\n" {
		t.Fatalf("unexpected recorded outcomes: %+v", entries)
	}
	if _, err := os.Stat(bound); err != nil {
		t.Fatalf("bound Markdown missing after failures: %v", err)
	}
}

func TestE2EBindOnlyMissingRecord(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "lab.md")
	log := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "bound.md")
	if err := os.WriteFile(input, []byte("```bash {terminal=true}\nprintf hello\n```\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCLI("lab-import", "--input", input, "--log", log, "--output", output, "--bind-only"); err == nil || !strings.Contains(err.Error(), "no ordered recording") {
		t.Fatalf("expected missing-record error, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output: %v", err)
	}
}
