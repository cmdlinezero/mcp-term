# CHANGE-007A — Optional exit-code markers in asciicast export

- Hide `[exit N]` markers by default for raw and Markdown-authored walkthrough casts.
- Add `export-asciinema --show-exit-code` to opt in to displaying exit codes.
- Preserve exit codes in canonical JSONL; export remains read-only.
- Add regression coverage for both presentation modes.
