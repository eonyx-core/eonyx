# Changelog

All notable changes to eonyx are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release binaries and their auto-generated notes are published on the
[Releases page](https://github.com/eonyx-core/eonyx/releases); GoReleaser
compiles the per-release notes from commit messages. This file is the
human-curated summary.

The agent was developed before this repository existed and its history begins
here, at a single commit. 1.0.0 is therefore the first release of eonyx as it
stands today, not the first line of code written.

## [1.0.1] - 2026-08-02

### Fixed

- `go install` works. The module path was declared as a bare `eonyx` while the
  README told people to install from the repository URL, and Go refuses a module
  whose declared path does not match where it was fetched from. The module is now
  `github.com/eonyx-core/eonyx`, matching the repository name.

## [1.0.0] - 2026-08-02

First release.

### Agent

- Headless agent loop: `eonyx run "prompt"` streams a response, executes tool
  calls, feeds the results back, and stops when the model asks for no more tools.
- Interactive terminal UI with streaming output, a command palette, model and
  session pickers, mouse-wheel scrolling, input history, and a full help screen.
- Any OpenAI-compatible API: OpenAI, OpenRouter, DeepSeek, Moonshot, and local
  servers such as Ollama, llama.cpp or LM Studio. No telemetry, no OAuth.
- Reasoning models: a separate thinking stream is shown as a collapsible block
  above the answer, toggled with Ctrl+R. Display-only, never persisted.
- Plan mode (`/plan`): a read-only "think first" phase in which only read-only
  tools run. When the turn ends, an approval dialog offers to implement the plan,
  enable edits, or keep planning.
- `task` sub-agent tool: delegates a self-contained job to a scoped child agent
  with read-only tools. The child cannot write, run commands, or spawn further
  sub-agents.

### Tools

- read, write, edit, multi_edit, ls, picker, glob, grep, bash, fetch, todo_write,
  and git_status / git_diff / git_log / git_commit.
- `bash` runs in-process through `mvdan.cc/sh`, so shell scripts work the same on
  Windows, Linux and macOS without a system shell.
- `dry_run` on write, edit and multi_edit previews the diff without touching disk.
- Word-level diff highlighting and syntax-highlighted code blocks.
- Optional file-tool sandbox (`permissions.sandbox`, off by default) confining
  file tools to the project root.

### Permissions

- Static policy: read-only tools always allowed, everything else needs an
  allowlist entry or `--yolo`.
- Interactive prompts in the TUI: a tool the policy would reject raises an
  "allow once / allow always / deny" dialog instead of failing outright.

### Sessions and memory

- JSONL session store with resume, full-text search (`eonyx sessions --search`),
  and forking (`/fork`) that branches a conversation without touching the original.
- `AGENTS.md` discovery up the directory tree.
- Automatic summarization of older turns as the context fills, with a
  context-pressure warning near 80%.

### MCP

- MCP client over stdio and HTTP on the official Go SDK.
- Server resources and prompts exposed as tools when the server supports them.
- Bearer-token and other headers for HTTP servers.
- Agent Skills: `SKILL.md` directories with progressive disclosure via
  `load_skill`.

### Scripting

- `run` reads a piped prompt from standard input (`cat bug.md | eonyx run`).
- `run --json` emits newline-delimited JSON events for CI and scripting.
- Distinct, documented exit codes (see `run --help`).
- Tool hooks: shell commands around tool calls, with input and results passed
  through the environment rather than interpolated into the script.

### Configuration and licensing

- Three merged configuration layers: global config, global data, project config.
  Unknown fields are fatal — eonyx refuses to start on a config it cannot honour.
- `--system-prompt`, `--log-file`, and a configurable retry policy and timeouts.
- Licence activation, verification and deactivation against eonyx.tech, bound to
  one machine per key. Server responses are Ed25519-signed over canonical JSON
  and the client pins the server's public key.

### Distribution

- Cross-platform releases via GoReleaser: linux, macOS and Windows on amd64 and
  arm64, with checksums.
- Container image published to `ghcr.io/eonyx-core/eonyx`.

[1.0.1]: https://github.com/eonyx-core/eonyx/releases/tag/v1.0.1
[1.0.0]: https://github.com/eonyx-core/eonyx/releases/tag/v1.0.0
