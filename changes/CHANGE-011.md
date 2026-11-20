# CHANGE-011 — Walkthrough presentation polish

- Adds `--operation-delay` to Markdown-bound `export-asciinema` walkthroughs (default `700ms`; range `0s`–`30s`) to control pacing between operations.
- Presents source-file writes with a visible destination label (for example, `[write file: main.go]`) and syntax-highlighted source instead of the internal `write-file` command.
- Shows a consistent shell prompt and updates its displayed working directory after successful, standalone `cd path` commands when the path can be determined safely. Complex shell expressions and ambiguous paths are not inferred.
- Preserves `typing="35ms"` character-by-character animation, existing syntax themes, and recorded stdout/stderr.
- Keeps JSONL recordings unchanged and exports asciinema v2; `--operation-delay` applies only to Markdown-bound walkthroughs, not JSONL-only exports.
- Adds regression coverage for pacing, source-file presentation, and directory-aware prompts, plus README usage documentation.
- Validation: local `go test ./...` passed after adding the missing standard-library `time` import to `cmd/root.go`. Race tests, vet, and visual playback should be confirmed before release.
