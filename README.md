# bd

A fast, minimal work item tracker. Drop-in replacement for [beads](https://github.com/steveyegge/beads) — same command surface, none of the bloat.

## Why

The original beads hangs during `bd create --parent`, ships 276K lines of Go, and requires Dolt for sync nobody uses. `bd` is a clean rewrite covering the exact CLI surface needed by workflow scripts, backed by a single SQLite file. Pure Go, zero CGo — builds and runs anywhere.

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

## Migrating from beads

If you have an existing `.beads/beads.db` from the original beads:

```bash
bd migrate    # backs up DB, converts old schema to new
```

The migration preserves all items, parent-child relationships, blocking dependencies, relations, notes, and config. A backup is created at `.beads/beads.db.bak`.

## Quick Start

```bash
bd init                                    # create .beads/beads.db
bd create --title="Fix bug" --type=task    # create a work item
bd create --title="Epic" --type=epic       # create an epic
bd create --title="Subtask" --parent=<id>  # create a child item
bd list                                    # list open items
bd show <id>                               # detail view
bd close <id>                              # mark done
bd                                         # dashboard
```

## Commands

### Core

| Command | Description |
|---------|-------------|
| `bd init` | Initialize database (`.beads/beads.db`) |
| `bd create -t "" --type= -p 2 --parent= -d ""` | Create item |
| `bd show <id> [--json] [--all]` | Show item details |
| `bd update <id> --status= --title= --type= --priority= --owner=` | Update fields |
| `bd update <id> --append-notes=""` | Add a note |
| `bd close <id>` | Set status to closed |
| `bd reopen <id>` | Set status to open |
| `bd delete <id>` | Permanently remove item and children |
| `bd list [--status=] [--type=] [--parent=] [--all]` | List items (hides closed by default) |
| `bd search "<query>"` | Full-text search on title and description |
| `bd ready [parent-id] [--json]` | Items with status open/in_progress and all blockers closed |
| `bd migrate` | Migrate old beads database to new schema |
| `bd docs` | Print command reference for agents and humans |
| `bd sync` | No-op (prints "nothing to sync") |

**Flag aliases:** `--title` / `-t`, `--priority` / `-p`, `--description` / `-d`

### Dependencies

| Command | Description |
|---------|-------------|
| `bd dep add <blocked> <blocker>` | Add blocking dependency |
| `bd dep remove <blocked> <blocker>` | Remove dependency |
| `bd dep relate <a> <b>` | Add relates_to relationship |

### Display

| Command | Description |
|---------|-------------|
| `bd` | Dashboard — epics with children and blocking relationships |
| `bd show <id>` | Detail view with children tree, deps, relations, notes |
| `bd deps` | Dependency chain DAG across epics |

## Data Model

- **Items**: id, title, description, type (task/bug/feature/chore/epic), status (open/in_progress/closed), priority (0-4, 0=critical), parent, owner
- **Dependencies**: blocking relationships between items
- **Relations**: non-blocking relationships (relates_to, duplicates, supersedes)
- **Notes**: append-only comments on items

Storage is a single SQLite file at `.beads/beads.db`, located by walking up from cwd (like `.git/`). Override with `BEADS_DIR` env var.

## ID Format

- Top-level: `{prefix}-{3 alphanum}` (e.g., `orc-4ho`)
- Children: `{parent}.{seq}` (e.g., `orc-4ho.1`, `orc-4ho.1.3`)
- Prefix defaults to the directory name where `bd init` is run

## License

MIT
