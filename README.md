# claudecode-go

Unix-first clean-room rewrite of a terminal coding assistant in Go with Bubble Tea.

## Current V1 Surface

- Bubble Tea chat REPL
- Slash commands: `/help`, `/clear`, `/resume`, `/model`, `/config`, `/permissions`, `/history`, `/quit`
- JSONL session transcripts and prompt history
- Anthropic and OpenAI-compatible provider adapters
- Built-in local tools:
  - `read_file`
  - `write_file`
  - `edit_file`
  - `glob`
  - `grep`
  - `shell`
  - `todo_write`
- Permission prompts with session or persisted allow/deny rules

## Commands

```bash
go run ./cmd/claudecode-go
go run ./cmd/claudecode-go chat
go run ./cmd/claudecode-go resume
go run ./cmd/claudecode-go config show
go run ./cmd/claudecode-go config set-secret default "$ANTHROPIC_API_KEY"
```

## Storage

- Config: `${XDG_CONFIG_HOME}/claudecode-go/config.yaml`
- State: `${XDG_STATE_HOME}/claudecode-go/`
- Sessions: `${XDG_STATE_HOME}/claudecode-go/sessions/*.jsonl`
- History: `${XDG_STATE_HOME}/claudecode-go/history.jsonl`

## Scope Notes

`leaksrc/` is retained only as a parity/reference inventory and remains ignored by Git.

Deferred for v1: web tools, MCP, plugins, LSP, Vim mode, remote bridge flows, multi-agent orchestration, and background jobs.
