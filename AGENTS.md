# AGENTS.md

All project guidance for AI coding agents lives in [CLAUDE.md](CLAUDE.md). Read it in full before making changes.

Short version:

- Go terminal text editor, modeless, single static binary, `CGO_ENABLED=0`, Linux/Windows/macOS.
- Hexagonal architecture: `internal/core/{domain,app,ports}` is pure and never imports tcell or `os`. Terminal, filesystem, clipboard and config code lives in `internal/adapters/*`. Only `cmd/sie` wires them.
- Every user action is a `Command` registered once and run through `Dispatch`, whether it comes from a shortcut, a menu or the prompt. Dispatch echoes it to the prompt line.
- Before finishing: `go vet ./...`, `go test -race ./...`, and the cross-compile matrix in CLAUDE.md.

Specialist agent definitions are in `.claude/agents/`.
