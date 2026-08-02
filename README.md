# eonyx

[![CI](https://github.com/eonyx-core/eonyx/actions/workflows/ci.yml/badge.svg)](https://github.com/eonyx-core/eonyx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/eonyx-core/eonyx?sort=semver)](https://github.com/eonyx-core/eonyx/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/eonyx-core/eonyx)](go.mod)
![Platforms](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-blue)

🖥️ A terminal-first AI coding agent, written in Go.  
🔌 It talks straight to any OpenAI-compatible API.  
🏠 By default it runs a local Ollama model.  
🔒 Nothing leaves your machine until you configure a cloud provider.  
📡 The only other call is licence verification against eonyx.tech.

## ✨ Features

- 💬 Interactive TUI with streaming, a command palette and model pickers.
- ⚙️ Headless `run` mode for scripts and pipelines.
- 🧰 Built-in tools: read, write, edit, ls, picker, glob, grep, bash, fetch.
- 🪟 The bash tool runs in-process via `mvdan.cc/sh/v3`. No system shell needed, Windows included.
- 🛡️ A permission gate. Reads are free, writes and commands need `--yolo` or an allowlist entry.
- 💾 Session history in JSONL, with resume. Context is summarised automatically as it fills up.
- 🧩 Agent Skills from local `SKILL.md` folders.
- 🔗 An MCP client over stdio and http.

## 🎯 Why eonyx

- 🏠 **Local-first.** No telemetry. No OAuth. Model traffic goes only where you point it.
- 📦 **One static binary.** No runtime. No interpreter. No `node_modules`.
- 🪟 **Windows-native bash.** Shell tools behave the same everywhere. No WSL needed.
- 🔌 **Bring your own endpoint.** Any cloud provider, or a local Ollama, llama.cpp or LM Studio.

## 📥 Install

Grab a prebuilt binary from [eonyx.tech](https://eonyx.tech/#downloads).  
Builds exist for linux, macOS and windows, on amd64 and arm64.

```bash
# Linux / macOS
curl -L https://eonyx.tech/downloads/eonyx-linux-amd64 -o eonyx
chmod +x eonyx && sudo mv eonyx /usr/local/bin/
eonyx --version
```

On Windows, download `eonyx-windows-amd64.exe` and put it on your `PATH`.

With a Go toolchain you can install from source:

```bash
go install github.com/eonyx-core/eonyx@latest
```

That build takes its version from VCS, not from a tag — see [Build](#-build).

A container image ships with every release:

```bash
docker run --rm -it -v "$PWD:/work" \
  -e DEEPSEEK_API_KEY ghcr.io/eonyx-core/eonyx:latest run "your prompt here"
```

## 🔑 Licence

The agent needs an activated licence.  
One licence binds one machine.  
Buy a key at [eonyx.tech](https://eonyx.tech).

```bash
eonyx license activate XXXX-XXXX-XXXX-XXXX   # bind this machine
eonyx license status                          # inspect / re-verify
eonyx license deactivate                      # free the key for another machine
```

Verification is Ed25519-signed with a pinned key.  
It runs at every startup, so the agent needs to reach eonyx.tech.

## 🔨 Build

Requires Go 1.26 or newer.

```bash
go build -o eonyx .   # build the binary
go test ./...         # run the tests
make build            # build into bin/ with the git version stamped in
```

`make build` injects the version from `git describe` through `-ldflags`.  
A plain `go build` falls back to the VCS version.  
Check it with `eonyx --version`.

## ⌨️ Usage

```bash
eonyx                                 # launch the interactive TUI (needs a terminal)
eonyx run "your prompt here"          # run the agent headlessly
eonyx run -m <model> --yolo "prompt"  # pick a model, skip the permission gate
eonyx sessions                        # list saved sessions
eonyx config                          # print the resolved configuration
eonyx info                            # print config/data/project paths
```

`run` takes `--model/-m`, `--session/-s <id>`, `--continue/-c` and `--yolo`.  
Streamed text goes to stdout.  
Tool activity and token counts go to stderr.

## 🧭 Configuration

Three layers merge, and the lower one wins.  
Global config, then global data, then project config.  
The project layer is found by walking up from the working directory for `eonyx.json` or `.eonyx.json`.  
Run `eonyx info` to see where those files live on your platform.

`api_key` and `base_url` may reference variables such as `$OPENAI_API_KEY`.  
They are expanded after the merge.

An unknown or malformed field is fatal.  
eonyx refuses to start rather than run a configuration it cannot fully honour.

[eonyx.example.json](eonyx.example.json) covers every supported field.

## 🚀 Releases

The version is the git tag, never a hardcoded string.

```bash
git tag -a v1.0.3 -m "eonyx 1.0.3"
git push origin v1.0.3
```

Pushing a `v*` tag triggers [release.yml](.github/workflows/release.yml).  
GoReleaser cross-builds linux, darwin and windows on amd64 and arm64.  
Then it publishes a release with checksums.

## 🏗️ Architecture

[How EONYX works](https://eonyx.tech/guide/eonyx-architecture/) walks through the agent loop, the tool registry and the permission model.

## 📄 Licence

Proprietary, all rights reserved.  
See [LICENSE](LICENSE).
