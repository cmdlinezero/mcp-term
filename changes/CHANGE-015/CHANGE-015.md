# CHANGE-015 — Self-demonstrating documentation

## Scope

- Add a reproducible executable Markdown walkthrough under `examples/`.
- Embed the generated GIF in the README using the repository `screenshots/` convention.
- Document the validated `lab-import` → JSONL → bound Markdown → `export-asciinema` → `agg` workflow, including `github-dark`, smooth reveal, and 64px GIF text.
- No Go implementation changes.

## Asset requirement

The approved GIF was generated and reviewed locally but was not uploaded here.
Copy the verified `mcp-term-self-demo.gif` into `screenshots/` before committing.
Do not commit the README reference without the corresponding GIF asset.

## Verification

1. `git apply --check CHANGE-015.patch` and `git apply CHANGE-015.patch`.
2. Copy the reviewed GIF to `screenshots/mcp-term-self-demo.gif`.
3. Run the documented commands from the repository root, using a clean `termcast-demo/` directory.
4. Check all commands exit zero, inspect the resulting GIF, and run `go test ./...`, `go test -race ./...`, `go vet ./...`, `git diff --check`.
5. Commit the patch, documentation, and GIF together on the feature branch.

## Release

Documentation-only change. No release tag until explicitly approved.
