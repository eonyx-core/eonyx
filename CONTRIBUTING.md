# Contributing to eonyx

Thanks for your interest in eonyx. This document covers how to build, test,
and submit changes.

> **License note:** eonyx is proprietary software (see [LICENSE](LICENSE)).
> By submitting a contribution you agree that it may be incorporated into the
> project under that license.

## Prerequisites

- Go 1.26 or newer.
- A terminal for the interactive TUI (`go run .`); the `run` subcommand works
  without one.

## Build and test

```bash
go build ./...     # build everything
go vet ./...       # vet
go test ./...      # run all tests
gofmt -l .         # list files that need formatting (must be empty)
make build         # build into bin/ with the git version stamped in
```

Before opening a pull request, make sure `go build ./...`, `go vet ./...`,
`go test ./...`, and `gofmt -l .` all pass — CI runs the same checks on Linux
and Windows, plus `go test -race` and `govulncheck`.

## Conventions

- Test files sit beside the code (`*_test.go`); add or update tests with your
  change.
- Tool handler errors are returned as content with an error flag, not as fatal
  Go errors, so the model can see and adapt to failures — follow this pattern.
- Dependencies are deliberately permissive and pure-Go. Do not add vendor LLM
  SDKs or Charm-forked tooling (catwalk, fantasy, charm SDK forks). Discuss any
  new dependency in an issue first.
- Keep the config example and the docs in sync when you add or change
  configuration fields.
- The build version is the git tag; never hardcode a version string (see
  `internal/version`).

## Commit messages

Short, imperative, English (e.g. "Fix status line encoding"). GoReleaser builds
the release changelog from commit subjects and drops `docs:`, `test:`, and
`chore:` prefixes.

## Submitting a change

1. Open an issue describing the bug or proposal (especially for larger changes)
   so the approach can be discussed before you invest time.
2. Create a topic branch, make focused commits, and keep the change scoped to
   one concern.
3. Ensure the checks above pass, then open a pull request that explains what and
   why, and links the issue.
