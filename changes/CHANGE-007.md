# CHANGE-007 — Markdown-authored walkthrough export

- Add optional `--lab` to `export-asciinema` for guided tutorial casts.
- Use bound Markdown as the source of step labels, instructional text, and command order.
- Resolve `file-id` and `command-id` against existing JSONL records; fail on missing,
  repeated, or mismatched references and altered command text.
- Preserve the JSONL-only exporter and read-only, exclusive-output behavior.
- Skip nonterminal code fences, YAML front matter, and Hugo shortcodes.
- Add regression tests for narrative playback and invalid bindings.

No command execution occurs during export.
