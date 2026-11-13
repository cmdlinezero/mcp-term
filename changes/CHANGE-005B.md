# CHANGE-005B — Markdown lab ingestion and recording generation

- Default `lab-import` executes `terminal=true` Bash/sh fences in document order
  through the canonical executor and writes a new JSONL recording.
- `--output` is optional: when provided, write a new Markdown copy whose
  `file-id` and `command-id` reference the generated JSONL execution records.
- `--bind-only` is optional read-only matching to existing JSONL and requires
  `--output`; no commands are executed in this mode.
- Record stdout, stderr, exit status, duration and unique ID for every fence.
  Continue after nonzero exits, report the failure count and return nonzero.
- Protect both JSONL and Markdown outputs from overwrites; never modify input.
- Preserve Hugo shortcodes, diagrams, hints, and original terminal script text.
- Support shell pipelines, redirections and quoting via `sh -c`.

## Safety

No sandbox flag or sandbox management. Commands run with the caller's
permissions. Run untrusted or destructive labs only in an appropriate test
environment. The JSONL recording is a new artifact, not an existing lookup
requirement for the default workflow.

## Verification

`go test ./...`, `go test -race ./...`, `go vet ./...`, `git diff --check`.
