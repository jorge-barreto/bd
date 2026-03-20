# bd

A fast, minimal work item tracker. Drop-in replacement for [beads](https://github.com/steveyegge/beads) — same command surface, none of the bloat.

## Why

The original beads hangs during `bd create --parent`, ships 276K lines of Go, and requires Dolt for sync nobody uses. `bd` is a clean rewrite that covers the exact CLI surface needed by workflow scripts, backed by a single SQLite file.

## Install

```bash
go install github.com/jorge-barreto/bd/cmd/bd@latest
```

Or build from source:

```bash
git clone https://github.com/jorge-barreto/bd.git
cd bd
go build -o bd ./cmd/bd
```

## Quick Start

```bash
bd init                          # create .beads/beads.db
bd create --title="Fix bug" --type=task
bd list
bd show <id>
bd close <id>
```

## Status

Under active development. See [SPEC.md](SPEC.md) for the full design.
