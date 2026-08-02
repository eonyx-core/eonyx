# eonyx

[![CI](https://github.com/eonyx-core/eonyx/actions/workflows/ci.yml/badge.svg)](https://github.com/eonyx-core/eonyx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/eonyx-core/eonyx?sort=semver)](https://github.com/eonyx-core/eonyx/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/eonyx-core/eonyx)](go.mod)
![Platforms](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-blue)

A terminal-first AI coding agent written in Go. It talks directly to
OpenAI-compatible model APIs — OpenAI, OpenRouter, DeepSeek, Moonshot, and local
servers like Ollama, llama.cpp, or LM Studio — with no telemetry and no OAuth.
By default it runs against a **local** Ollama model, so no data leaves the
machine until you explicitly configure a cloud provider. Single process, runs
locally; the only other network call is EONYX license activation/verification
against eonyx.tech, which runs at every startup (internet required).

## Features

- Interactive TUI chat with streaming, a command palette, and model/session
  pickers.
- Headless `run` mode for scripts and pipelines.
- Built-in tools: read, write, edit, ls, picker, glob, grep, bash, fetch. The
  bash tool runs in-process (via `mvdan.cc/sh/v3`), so it works on Windows
  without a system shell.
- A permission gate: read-only tools are always allowed; writes and commands need
  an allowlist entry or `--yolo`.
- Session history (JSONL) with resume, and automatic context summarization when
  history approaches the model's context window.
- Agent Skills (local `SKILL.md` folders) and an MCP client (stdio + http) for
  external tools.

## Why eonyx?

Compared to other terminal coding agents (aider, crush, opencode):

- **Local-first, no telemetry, no OAuth.** Model traffic goes straight to the
  endpoint you configure — a local Ollama model by default. The only other
  network call is license activation/verification against eonyx.tech.
- **A single static binary.** One cross-compiled Go executable per platform, no
  runtime, interpreter, or `node_modules` to install.
- **Windows-native bash.** The bash tool runs in-process via `mvdan.cc/sh/v3`,
  so shell tools behave the same on Windows as on Linux and macOS without WSL or
  a system shell.
- **Bring your own endpoint.** Any OpenAI-compatible API works — OpenAI,
  OpenRouter, DeepSeek, Moonshot, or a local Ollama / llama.cpp / LM Studio
  server — so you can run fully offline against a local model.

## Install

Download a prebuilt binary from [eonyx.tech](https://eonyx.tech/#downloads):
linux (amd64), macOS (Apple Silicon / Intel), and windows (x64).

```bash
# Linux / macOS
curl -L https://eonyx.tech/downloads/eonyx-linux-amd64 -o eonyx
chmod +x eonyx && sudo mv eonyx /usr/local/bin/
eonyx --version
```

On Windows, download `eonyx-windows-amd64.exe` and put it on your `PATH`.

## License activation

Running the agent requires an activated license (one license = one machine).
Buy a key at [eonyx.tech](https://eonyx.tech), then:

```bash
eonyx license activate XXXX-XXXX-XXXX-XXXX   # bind this machine
eonyx license status                          # inspect / re-verify
eonyx license deactivate                      # free the key for another machine
```

Verification is signed (Ed25519, pinned server key) and runs at every startup —
an internet connection to eonyx.tech is required to launch the agent.

With a Go toolchain you can also install from source:

```bash
go install github.com/eonyx-core/eonyx@latest
```

(A `go install` build reports the version the Go toolchain embeds from VCS
rather than a clean tag; see [Build](#build) for the tag-stamped build.)

A container image is published to GitHub Container Registry on each release:

```bash
docker run --rm -it -v "$PWD:/work" \
  -e DEEPSEEK_API_KEY ghcr.io/eonyx-core/eonyx:latest run "your prompt here"
```

## Build

Requires Go 1.26+.

```bash
go build -o eonyx .   # build the binary
go test ./...            # run the tests
make build               # build into bin/ with the git version stamped in
```

`make build` injects the version from `git describe` via `-ldflags`; a plain
`go build` falls back to the version the Go toolchain embeds from VCS. Check it
with `eonyx --version`.

## Usage

```bash
eonyx                                 # launch the interactive TUI (needs a terminal)
eonyx run "your prompt here"          # run the agent headlessly
eonyx run -m <model> --yolo "prompt"  # pick a model, skip the permission gate
eonyx sessions                        # list saved sessions
eonyx config                          # print the resolved configuration
eonyx info                            # print config/data/project paths
```

`run` flags: `--model/-m`, `--session/-s <id>`, `--continue/-c` (resume the most
recent session), `--yolo` (allow all tools). Streamed text goes to stdout; tool
activity and token/session info go to stderr.

## Configuration

Configuration merges three layers (lower overrides higher): global config →
global data → project config, found by walking up from the working directory for
`eonyx.json` or `.eonyx.json`. Provider `api_key`/`base_url` may reference
environment variables (e.g. `$OPENAI_API_KEY`), expanded after the merge. Run
`eonyx info` to see where these files live on your platform.

Unknown or malformed config fields are a fatal error — eonyx refuses to start
rather than run a configuration it cannot fully honor.

See [eonyx.example.json](eonyx.example.json) for a complete example
covering every supported field (providers, models, permissions, skills, MCP
servers, hooks, and the report database).

## Releases

The version is the git tag — never hardcoded. To cut a release:

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

Pushing a `v*` tag triggers the GitHub Actions workflow
([.github/workflows/release.yml](.github/workflows/release.yml)), which runs
GoReleaser ([.goreleaser.yaml](.goreleaser.yaml)) to cross-build binaries for
linux/darwin/windows (amd64/arm64), generate a changelog, and publish a GitHub
Release with checksums.

## Architecture & roadmap

See [CLAUDE.md](CLAUDE.md) for a package-by-package architecture overview.

## License

Proprietary — all rights reserved. See [LICENSE](LICENSE).
