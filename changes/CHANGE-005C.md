# CHANGE-005C — End-to-end recording regression tests

## Scope

Add automated CLI-level integration tests for the CHANGE-005A/005B pipeline:
Markdown terminal fences → canonical shell executor → JSONL → optional
bound Markdown → read-only asciicast v2 export. No live Docker daemon is needed.

## Acceptance criteria

- Two ordered commands, including a shell pipeline, produce two distinct JSONL IDs,
  expected stdout, timestamps, durations and successful exit codes.
- Bound Markdown preserves Hugo shortcodes, diagrams and original source bytes,
  while each terminal fence references its JSONL execution ID.
- Read-only `--bind-only` produces equivalent references without execution.
- Exported asciicast v2 contains the recorded output; export never re-executes.
- Existing JSONL, Markdown and cast files cannot be overwritten accidentally.
- A failed command records stderr and exit status, subsequent commands still run,
  and the CLI reports the nonzero outcome after writing both artifacts.
- Missing recordings in bind-only mode produce an error without creating output.

## Verification

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and
`git diff --check`. Tests use POSIX `/bin/sh` and skip on systems where it is
unavailable. The Docker tutorial sandbox acceptance test remains a separate
manual/environment-dependent check.
