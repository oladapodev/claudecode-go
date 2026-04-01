# V1 Parity Matrix

This document keeps the rewrite inside the agreed v1 boundary.

## Retained In V1

- Interactive chat REPL
- Slash commands inside the REPL
- Streaming model output
- Local JSONL session transcripts
- Prompt history and `/resume`
- Config and provider profiles
- Permission prompts and persisted allow/deny rules
- Built-in tools:
  - `read_file`
  - `write_file`
  - `edit_file`
  - `glob`
  - `grep`
  - `shell`
  - `todo_write`

## Explicitly Deferred

- Web search and web fetch
- MCP servers, resources, auth, and tool pooling
- Plugins, skills, marketplaces
- LSP and code intelligence
- Multi-agent orchestration and background jobs
- Remote bridge / daemon flows
- Desktop/mobile/voice modes
- Vim mode and configurable keymaps
- Custom terminal renderer parity
- Cloud sync, analytics, growth experiments, and internal-only commands
