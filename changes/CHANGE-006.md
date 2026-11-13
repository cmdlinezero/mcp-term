# CHANGE-006 — Cloud Build and initial release preparation

## Scope

Prepare TermCast MCP for its first annotated SemVer release, `v0.1.0`, using
Google Cloud Build for commit-triggered verification on `main`. Do not add
GitHub Actions, GitHub Releases automation, or automatic tagging.

## Changes

- Add `cloudbuild.yaml` with sequential tests, race detector, vet, and build.
- Use Go 1.23 to match the minimum version declared in `go.mod`.
- Correct the public module path from `github.com/example/mcp-term` to
  `github.com/cmdlinezero/mcp-term`, including internal imports.
- Document the externally configured `main` branch trigger and manual
  annotated-tag release procedure in README.
- Remove the GitHub Releases badge; no GitHub Release is required.

## Acceptance criteria

- `git apply --check` and `git diff --check` pass.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...` pass.
- A push to `main` triggers a successful Google Cloud Build run using
  `cloudbuild.yaml`; this must be confirmed in the user's Google Cloud project.
- The verified commit has a clean working tree before tagging.
- Create and push annotated `v0.1.0` only after successful Cloud Build.
- The build configuration must not publish or move tags.

## Notes

Cloud Build trigger, permissions, and logging configuration are managed outside
this repository. Cloud Build must have network access to fetch Go modules.
The binary built in `/workspace` is ephemeral and is not published.
