# CHANGE-010 — Built-in syntax themes

- Adds `--theme dracula|monokai|github-dark` and `--no-highlight` to the walkthrough export command.
- Uses ANSI true-color escapes only in cast command-input events. JSONL recordings and recorded stdout/stderr are not modified.
- Supports Bash command token categories and Go source-file keywords, strings, comments, flags, and numbers.
- Preserves the existing uncolored default and the existing exported file format.
- Theme selection requires `--lab`; no custom palette configuration in this change.
