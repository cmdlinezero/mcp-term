package internal

import (
	"strings"
	"testing"
)

func TestHighlightThemes(t *testing.T) {
	input := "go run . -port 8081\n"
	for _, theme := range []string{"dracula", "monokai", "github-dark"} {
		got := HighlightCastInput(input, theme)
		if !strings.Contains(got, "\x1b[38;2;") {
			t.Errorf("%s: missing ANSI color", theme)
		}
		if strings.ReplaceAll(got, "\x1b[0m", "") == input {
			t.Errorf("%s: no highlighting", theme)
		}
	}
	if HighlightCastInput(input, "") != input {
		t.Fatal("plain output changed")
	}
	if err := ValidateHighlightTheme("invalid"); err == nil {
		t.Fatal("invalid theme accepted")
	}
}
func TestHighlightSource(t *testing.T) {
	input := "write-file main.go\npackage main\nfunc main() { // hello\n}\n"
	got := HighlightCastInput(input, "dracula")
	if !strings.Contains(got, "\x1b[38;2;") || !strings.Contains(got, "package") {
		t.Fatal("source not highlighted")
	}
}
