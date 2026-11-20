# CHANGE-009 — Markdown fence compatibility and source materialization

- Accept both braced and unbraced `terminal=true` fences, including quoted `terminal="true"`.
- Normalize bound fence attributes to braced form and replace stale `file-id`/`cmd-id` with canonical `file-id`/`command-id` bindings.
- Treat non-shell terminal fences as source-file writes, requiring `file="relative/path"` and `overwrite="true"` for replacement.
- Record source writes in JSONL so read-only binding and walkthrough export can match them.
- Track standalone `cd` commands across lab blocks; constrain destination writes to the lab workspace.
- Preserve existing JSONL-only and shell-only behavior.

## Source-file example

````markdown
```go {terminal="true" file="main.go" copy="true"}
package main
```
````

`lab-import` executes shell commands and writes source files; `--bind-only` and `export-asciinema` remain read-only.

**Limits:** Shell invocations remain independent, so exported environment variables do not persist between blocks. A standalone `cd directory` persists, but compound shell directory changes do not. Each shell command retains the default 30-second timeout. Run labs only in a controlled workspace and review executable blocks before importing.
