# Changelog

All notable changes to eonyx are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims
to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release binaries and their auto-generated notes are published on the
[Releases page](https://github.com/eonyx-core/eonyx.cli/releases); GoReleaser
compiles the per-release changelog from commit messages. This file tracks the
human-curated summary.

## [1.1.0] - 2026-08-02

### Added

- Reasoning model support: a model's separate thinking stream
  (`reasoning_content` / `reasoning`) is parsed and shown as a collapsible
  "thinking" block above the answer — collapsed by default, toggled with Ctrl+R
  in the TUI (`run` prints it under `[thinking]`; `--json` adds `reasoning`
  events). It is display-only and never persisted, and models without reasoning
  show nothing.
- Prompt cache reporting: the provider's cached-prefix tokens
  (OpenAI `prompt_tokens_details.cached_tokens` / DeepSeek
  `prompt_cache_hit_tokens`) are parsed and shown in the TUI header and the
  `run` token summary (`--json` adds `cached_tokens`). eonyx already keeps a
  stable prompt prefix, so these providers cache it automatically.
- README demo: a TUI screenshot and an asciinema recording
  ([`docs/demo.cast`](docs/demo.cast)), both generated from the real rendering
  via the `democast` build tag.
- Syntax highlighting in code blocks (via `alecthomas/chroma`): keywords,
  strings, numbers, comments, and function names are colored, with the language
  shown as a badge on the block's top border. Unknown languages render plain.
- Plan mode (`/plan`): a read-only "think first" phase. While on, only read-only
  tools run (read, ls, glob, grep, git status/diff/log, fetch, task, todo_write)
  and every writing or command tool is blocked. When a plan-mode turn ends, an
  approval dialog offers to implement the plan, enable edits, or keep planning.
  A PLAN badge shows in the header.
- `task` sub-agent tool: delegates a self-contained job to a scoped child agent
  that runs its own loop with read-only tools (read, ls, glob, grep, fetch) and
  returns a concise result. The child cannot write, run commands, or spawn
  further sub-agents.
- Interactive permission prompts in the TUI: a tool the static policy would
  reject now raises an "allow once / allow always / deny" dialog instead of
  failing outright; "allow always" grants it for the rest of the session. Esc
  denies. `--yolo` and the allowlist still bypass the prompt.

- `run` reads the prompt from standard input when it is piped and no prompt
  argument is given (`cat bug.md | eonyx run`).
- `run --json` emits structured newline-delimited JSON events (text, tool
  start/end, usage, final result) for scripting and CI.
- `--system-prompt` flag and `system_prompt` config field to override the
  generated system prompt, in addition to `AGENTS.md` discovery.
- `--log-file` flag and `log_file` config field to direct the log to an explicit
  path instead of the per-project default.
- Configurable retry policy and timeouts (`retry.max_retries`,
  `retry.base_delay`, `retry.response_header_timeout`) instead of hardcoded
  constants.
- `multi_edit` tool: several replacements to one file in a single atomic call.
- Git tools: `git_status`, `git_diff`, `git_log`, and `git_commit` as first-class
  tools (the read-only three run without a permission prompt).
- Full-text search across sessions: `eonyx sessions --search <text>` and
  `/sessions <text>` in the TUI session picker.
- Session fork: `Store.Fork` and the `/fork` TUI command branch a conversation
  into a new session, leaving the original untouched.
- Word-level diff highlighting: single-line replacements highlight only the
  changed span, on top of the +/- line colouring.
- TUI: the footer distinguishes "Thinking" from "Running <tool>" and shows a
  rough live tokens/sec rate while streaming.
- TUI: a compact header and welcome banner for narrow terminals (<60 columns).
- `dry_run` option on `write`, `edit`, and `multi_edit` to preview the diff
  without touching disk.
- TUI: session duration in the header, and a `/timestamps` toggle that shows a
  subtle per-turn timestamp.
- TUI: Ctrl+O toggles between collapsed tool-output summaries and their full
  text (uncapped diffs).
- `todo_write` tool: the model can record and update a task list to track
  multi-step work.
- Optional file-tool sandbox (`permissions.sandbox`, off by default): confines
  read/write/edit/multi_edit/ls to the project root.
- TUI: inline autocomplete for `/` commands and `@` file mentions (Tab accepts,
  arrows select, Esc dismisses). Typing `/` now completes inline; the modal
  command palette moves to Ctrl+P only.
- TUI: type-to-filter in the file picker (`/add`), narrowing the list as you
  type with backspace to widen; directories show their item count.
- MCP: resources and prompts are exposed as tools
  (`<server>_list_resources`/`_read_resource`,
  `<server>_list_prompts`/`_get_prompt`) when the server supports them.
- Markdown rendering: blockquotes, horizontal rules, strikethrough, and OSC 8
  clickable links in the transcript.
- Auth for HTTP MCP servers via a `headers` field (e.g. a bearer token).
- Tests for the `internal/logging` and `internal/version` packages, and a
  coverage-reporting job in CI.
- TUI: input history recall with the up/down arrows.
- TUI: mouse-wheel scrolling of the transcript.
- TUI: full help screen (`?` / `/help`) listing every keybinding and command.
- TUI: scroll-position indicator in the transcript viewport.
- TUI: context-pressure warning that highlights the context gauge near ~80%,
  before auto-summarization kicks in.

### Changed

- `run` exits with distinct, documented status codes (see `run --help`).

## [1.0.0] - 2026-06-23

First public release. Includes the headless agent loop, the interactive TUI,
the built-in tool set (read, write, edit, ls, picker, glob, grep, bash, fetch),
the permission gate, JSONL session persistence with resume and automatic context
summarization, Agent Skills, the MCP client (stdio + http), tool hooks, and
GoReleaser-driven cross-platform releases.

## [0.1.0] - 2026-06-22

Initial tagged pre-release.

[1.1.0]: https://github.com/eonyx-core/eonyx.cli/releases/tag/v1.1.0

The 1.0.0 and 0.1.0 entries above are kept for the record. They predate the move
to this repository, whose history starts at a single commit, so there are no tags
to compare against.
