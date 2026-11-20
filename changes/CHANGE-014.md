# CHANGE-014 — Framed callouts and smooth source reveal

- Frames `info`, `warn`, and `hint` blocks with compact labeled headings and consistent 40-column closing rules, retaining green/red/cyan labels when highlighting is enabled and readable plain-text boundaries otherwise.
- Adds `--source-reveal lines|smooth` for bound Markdown exports. `lines` (default) preserves existing behavior and `--source-line-delay`; `smooth` emits short UTF-8-safe, ANSI-preserving source chunks at 25ms intervals to improve GIF frame cadence with `agg`.
- Keeps `tutorial`/`demo` selection unchanged; demo retains framed warnings but excludes info and hints.
- Preserves recorded output, command status, immutable JSONL, and asciinema v2 compatibility.
- Adds regression coverage for framed callouts, smooth chunking, ANSI/Unicode preservation, and invalid reveal modes.
- Note: `agg` frame rate and GIF scrolling behavior also affect perceived smoothness; chunking reduces line-at-once jumps but does not guarantee pixel-smooth terminal scrolling.
