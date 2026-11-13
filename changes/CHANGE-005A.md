# CHANGE-005A — Read-only JSONL playback export

Adds `mcp-term export-asciinema` for source-agnostic asciicast v2 export,
record lookup by execution ID, and regression tests for validation, duplicate
IDs, corrupt JSONL, output protection, and read-only behavior.

This subphase does not implement Markdown lab ingestion or change the existing
`replay` re-execution semantics. Exported timing is approximate because JSONL
records do not contain individual output event timestamps.

Verification (run independently before commit):

```sh
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

## Pre-commit documentation and licensing update

Public project name: **TermCast MCP**. The executable remains `mcp-term`;
MCP protocol and JSONL format are unchanged. README now includes GitHub
badges, architecture, record format, security, test and playback guidance.
Adds an MIT LICENSE with copyright attributed to cmdlinezero contributors.
Confirm license ownership/permission before publishing.
