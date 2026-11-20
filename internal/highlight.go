package internal

import (
	"fmt"
	"regexp"
	"strings"
)

// Built-in ANSI palettes. Color is applied only to generated cast input, never to recordings.
type highlightPalette struct{ command, keyword, flag, quoted, comment, number string }

var palettes = map[string]highlightPalette{
	"dracula":     {"#8be9fd", "#ff79c6", "#bd93f9", "#f1fa8c", "#6272a4", "#ffb86c"},
	"monokai":     {"#66d9ef", "#f92672", "#ae81ff", "#e6db74", "#75715e", "#fd971f"},
	"github-dark": {"#79c0ff", "#ff7b72", "#d2a8ff", "#a5d6ff", "#8b949e", "#ffa657"},
}

func ValidateHighlightTheme(theme string) error {
	if theme == "" {
		return nil
	}
	if _, ok := palettes[theme]; !ok {
		return fmt.Errorf("unknown theme %q (choose dracula, monokai, github-dark)", theme)
	}
	return nil
}

var tokenPattern = regexp.MustCompile(`(?m)//[^\n]*|#[^\n]*|"(?:\\.|[^"\\])*"|'[^']*'|\$\{?[A-Za-z_][A-Za-z0-9_]*\}?|--?[A-Za-z][A-Za-z0-9_-]*(?:=[^\s]+)?|\b[0-9]+\b|\b[A-Za-z_][A-Za-z0-9_]*\b`)
var goKeywords = map[string]bool{"package": true, "import": true, "func": true, "return": true, "var": true, "const": true, "type": true, "struct": true, "interface": true, "if": true, "else": true, "switch": true, "case": true, "for": true, "range": true, "defer": true, "go": true, "select": true, "map": true, "chan": true, "nil": true, "true": true, "false": true}

func ansiColor(hex string) string {
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// HighlightCastInput colors command/source text without changing the recorded bytes.
// The source-file representation is the existing write-file <path>\n<content> format.
func HighlightCastInput(script, theme string) string {
	p, ok := palettes[theme]
	if !ok {
		return script
	}
	source := strings.HasPrefix(script, "write-file ")
	lang := "bash"
	if source {
		lang = "go"
	}
	var out strings.Builder
	pos := 0
	commandSeen := false
	for _, loc := range tokenPattern.FindAllStringIndex(script, -1) {
		out.WriteString(script[pos:loc[0]])
		token := script[loc[0]:loc[1]]
		color := ""
		switch {
		case strings.HasPrefix(token, "#") || strings.HasPrefix(token, "//"):
			color = p.comment
		case strings.HasPrefix(token, "\"") || strings.HasPrefix(token, "'"):
			color = p.quoted
		case strings.HasPrefix(token, "$"):
			color = p.flag
		case strings.HasPrefix(token, "-"):
			color = p.flag
		case token[0] >= '0' && token[0] <= '9':
			color = p.number
		case lang == "go" && goKeywords[token]:
			color = p.keyword
		case lang == "bash" && !commandSeen:
			color = p.command
			commandSeen = true
		case lang == "bash" && (token == "export" || token == "cd" || token == "echo"):
			color = p.keyword
		}
		if color != "" {
			out.WriteString(ansiColor(color))
			out.WriteString(token)
			out.WriteString("\x1b[0m")
		} else {
			out.WriteString(token)
		}
		pos = loc[1]
	}
	out.WriteString(script[pos:])
	return out.String()
}
