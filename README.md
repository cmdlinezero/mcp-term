# Terminal Runner MCP Server

A Go MCP stdio server that executes local programs, records JSONL sessions, and replays them into separate logs.

## Requirements

- Go 1.23+
- Network access once to download `github.com/spf13/cobra` (and its dependencies)

## Build and test

```sh
go mod tidy
go test ./...
go build -o mcp-term .
```

## Usage

```sh
./mcp-term serve --log session.jsonl
./mcp-term replay --log session.jsonl --output replay-session.jsonl
git diff --no-index session.jsonl replay-session.jsonl || true
```

The replay output must not exist already. Each replayed command receives a fresh execution ID and timestamp, so raw logs will differ even if command output matches. For meaningful comparisons, normalize the `id`, `timestamp`, and `duration` fields first.

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

Run regression tests with `go test ./...`, `go test -race ./...`, and `go vet ./...`.
