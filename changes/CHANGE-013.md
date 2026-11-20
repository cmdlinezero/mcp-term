# CHANGE-013 — Presentation modes and callout prefixes

- Adds `--presentation tutorial|demo` to bound Markdown walkthrough export; `tutorial` remains the default.
- Demo retains section step labels, task headings associated with recorded terminal operations, executable code, recorded output, and warning callouts; removes explanatory prose and sections without commands.
- Renders fenced `info`, `warn`, and `hint` blocks in tutorial mode with `INFO:`, `WARN:`, and `HINT:` prefixes. Demo retains warnings but omits info and hints.
- Prefixes use green, red, and cyan ANSI colors when a syntax theme is active, and readable plain text without a theme.
- Reuses existing Markdown step shortcodes and headings; no new authoring markers or recording schema changes.
- Keeps JSONL recordings immutable, bound command validation intact, and asciinema v2 output unchanged.
- Adds regression tests for both modes, themed/plain callouts, task selection, and invalid mode rejection.
