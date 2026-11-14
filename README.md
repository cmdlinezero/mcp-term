# TermCast MCP

[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Record once. Replay anywhere.**

TermCast MCP is a Go Model Context Protocol (MCP) stdio terminal server. Its `mcp-term` executable runs commands, captures stdout, stderr, exit status and duration in append-only JSONL, and supports both command re-execution and read-only asciinema-compatible playback export.

**Repository:** https://github.com/cmdlinezero/mcp-term

## Requirements

- Go 1.23+
- Network access once to download `github.com/spf13/cobra` (and its dependencies)

## Build and test

```sh
go mod tidy
go test ./...
go build -o mcp-term .
```

## Architecture

```text
MCP client (e.g. DejaVu)
        | JSON-RPC over stdio
        v
   mcp-term serve
        | run_command
        v
   Command execution --> session.jsonl
                             |         |
                             |         +--> export-asciinema --> session.cast --> asciinema play
                             +--> replay (re-executes commands)
```

## Usage

```sh
./mcp-term serve --log session.jsonl
./mcp-term replay --log session.jsonl --output replay-session.jsonl
git diff --no-index session.jsonl replay-session.jsonl || true
```

The replay output must not exist already. Each replayed command receives a fresh execution ID and timestamp, so raw logs will differ even if command output matches. For meaningful comparisons, normalize the `id`, `timestamp`, and `duration` fields first.

## Recording format

Each JSONL line is a separate execution record. For example:

```json
{"id":"04e824b195011ad7","timestamp":"2026-11-12T10:49:11.675009Z","command":"go","args":["version"],"stdout":"go version go1.26.7 darwin/arm64\n","stderr":"","exit_code":0,"duration":"43.408ms"}
```

Execution IDs are generated for each run, including repeated identical commands. A Markdown lab or other client can reference an existing record using its JSONL filename and `id`; the read-only exporter does not depend on the source of the recording.

## MCP client configuration

Example configuration (adapt the key structure to your MCP client):

```json
{
  "mcpServers": {
    "terminal-runner": {
      "command": "/absolute/path/to/mcp-term",
      "args": ["serve", "--log", "/absolute/path/to/session.jsonl"]
    }
  }
}
```

The server accepts one JSON-RPC message per line on stdin and writes responses to stdout. It supports `initialize`, `notifications/initialized`, `ping`, `tools/list`, and `tools/call`. Diagnostics and replay summaries go to stderr.

## Tool call example

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_command","arguments":{"command":"go","args":["version"]}}}
```

The tool response contains the JSON-serialized command record as text content. Execution errors are logged and reported using `isError` rather than crashing the server.

## Security and reproducibility

**This server is not a sandbox.** Anyone allowed to invoke `run_command` can run programs with the server process's OS privileges. Use only in a trusted, isolated environment (e.g. a disposable container or VM with limited filesystem access and no secrets). Direct `os/exec` invocation does not implicitly run a shell, but commands can still launch shells themselves. The default per-command timeout is 30 seconds, and each captured stream is limited to 1 MiB. Output truncation is not currently indicated in the log. Child processes may outlive the parent on some platforms.

Replay repeats the command and arguments, not a complete snapshot of environment, working directory, filesystem, network, or dependencies. Therefore identical output is not guaranteed. Command output and arguments may contain secrets; JSONL logs are created with mode 0600 on Unix-like systems.

## Read-only terminal playback export (CHANGE-005A)

Export an existing mcp-term JSONL recording to asciicast v2 without re-executing commands:

```sh
./mcp-term export-asciinema --log session.jsonl --output session.cast
asciinema play session.cast
```

The output file must not already exist. Export rejects duplicate execution IDs,
invalid timestamps/durations, and malformed JSONL. The exported cast includes
recorded command lines, stdout, stderr, and exit codes. It uses bounded delays
between commands, but **cannot reconstruct per-byte timing or stdout/stderr
interleaving**, because the canonical JSONL records contain only aggregated
streams and command-level timestamps. The export does not run commands.

`mcp-term replay` is a different, potentially side-effecting operation: it
**re-executes** recorded commands. Only use it with trusted, safe recordings.

## Testing

```sh
go test ./...
go test -race ./...
go vet ./...
```

## Google Cloud Build and releases (CHANGE-006)

The repository uses a Google Cloud Build trigger for commits to `main`, not
GitHub Actions. Configure the existing trigger in Google Cloud to use the
repository's `cloudbuild.yaml` (configuration type: Cloud Build configuration
file). The trigger itself, its branch filter and service account are managed
in Google Cloud, not in this repository.

Cloud Build runs `go test ./...`, `go test -race ./...`, `go vet ./...`, and
`go build` sequentially using Go 1.23, matching `go.mod`. It requires access
to download the Go module dependencies; the compiled binary is a temporary
build output, not a published artifact. The build must pass before a release
is tagged. Local verification remains:

```sh
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

The Go module is `github.com/cmdlinezero/mcp-term`. The initial intended
version is `v0.1.0`; **do not create the tag until the CHANGE-006 commit is
pushed to `main` and its Cloud Build succeeds**. From a clean checkout of the
verified commit:

```sh
git switch main
git pull --ff-only
git status --short
git log -1 --oneline
# Confirm the commit above is the commit that passed Cloud Build.
git tag -a v0.1.0 -m "TermCast MCP v0.1.0 — initial release (CHANGE-005A through CHANGE-006)"
git push origin v0.1.0
```

Tags are annotated and pushed manually; Cloud Build does not tag or publish
releases. Do not reuse or move a published version tag.

## License

TermCast MCP is distributed under the [MIT License](LICENSE). Copyright (c) 2026 cmdlinezero contributors. Preserve the copyright and permission notices when redistributing the software.

## Markdown lab ingestion (CHANGE-005B)

`lab-import` is an optional adapter for Hugo-style Markdown labs. It finds
`bash`/`sh` fences with `terminal=true` (including those inside `{{< step >}}`
shortcodes), leaves other Markdown unchanged, and replaces legacy `cmd-id`
attributes with real `command-id` values from JSONL execution records.

**Execute and record (default):**

```sh
./mcp-term lab-import --input import-export.md --log docker-lab.jsonl
```

The importer executes each `terminal=true` `bash`/`sh` fence in document order
through the canonical executor and writes a **new** JSONL recording. It records
stdout, stderr, exit status, duration and execution ID, including failures.
Nonzero exits do not prevent subsequent fences from running; after recording all
commands, the CLI returns an error if any command exited nonzero. The JSONL
output must not already exist; use a fresh path for each run.

**Also generate bound Markdown (optional):**

```sh
./mcp-term lab-import --input import-export.md --log docker-lab.jsonl \
  --output import-export-bound.md
```

The original Markdown is never modified. The generated copy updates only
terminal fence opening lines to reference the new JSONL IDs; the output file
must not already exist.

**Bind existing recordings without execution (optional):**

```sh
./mcp-term lab-import --input import-export.md --log docker-lab.jsonl \
  --output import-export-bound.md --bind-only
```

This matches commands **in order** against existing recordings, failing if a
match is absent. `--bind-only` requires `--output` and never executes commands.

Execution uses `sh -c` for each terminal fence to support pipes, redirects,
and shell quoting. This is **not sandboxed**: scripts run with the current
process permissions and can modify files, access secrets or delete resources.
Review the lab and use an appropriate disposable test environment. Markdown
is an optional adapter; direct MCP tool calls remain supported.

The generated Markdown contains references such as:

````markdown
```bash {terminal=true copy=true file-id="session.jsonl" command-id="04e824b195011ad7"}
go version
```
````

`file-id` is the JSONL filename, not a stable lab ID. `command-id` is the
actual JSONL record `id`, not the old `cmd-id`. Keep the referenced JSONL file
available to the Markdown renderer. The importer does not implement a renderer.

### Automated end-to-end regression (CHANGE-005C)

`go test ./...` includes CLI-level integration tests for Markdown command
execution, JSONL recording integrity, bound Markdown references, read-only
binding, asciicast v2 export, nonzero exit handling and overwrite protection.
The tests run short POSIX shell commands in temporary directories; no Docker
daemon or network access is required. On systems without `/bin/sh`, the
shell-dependent tests skip.

```sh
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

## Markdown-authored walkthrough export (CHANGE-007)

Use a bound Markdown lab as the **presentation source** and an existing JSONL
recording as the **execution-result source**:

```sh
./mcp-term export-asciinema --lab test/import-export-bound.md --log docker-lab.jsonl --output docker-lab-demo.cast
asciinema play docker-lab-demo.cast
```

The `--lab` option is optional. Without it, the original JSONL-only cast export
is unchanged. With it, Markdown step labels, headings, paragraphs, and list
instructions appear before their associated recorded commands. The exporter
uses each terminal fence's existing `file-id` and `command-id` attributes to
resolve output from the JSONL; command order follows the Markdown, not the log.
Authors may change narration, reorder or omit bound commands, and add notes
without re-executing the lab. The command text inside a bound fence must still
match the recorded command; mismatches fail rather than misrepresent execution.

Nonterminal fenced blocks (such as Mermaid and hints), Hugo shortcodes, and YAML
front matter are omitted. This is a plain-terminal interpretation, not a full
Markdown renderer. The cast is read-only; no commands are executed. Output is
created exclusively and existing files are never overwritten.

Exit-code markers are omitted from both raw and lab-aware casts by default. Use `--show-exit-code` with `export-asciinema` to display them when demonstrating failures.
