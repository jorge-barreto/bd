# bd — Drop-in Work Item Tracker

## Context

The original `bd` (steveyegge/beads) hangs during `bd create --parent`, uses Dolt for sync we don't need, and is 276K lines of Go. We need a fast, reliable drop-in replacement that covers exactly what orc's workflow scripts and agent prompts use. No sync, no daemon, no JSONL, no Dolt.

Lives at `~/work/bd`. Binary is `bd`. Same command surface as the original where it overlaps.

## Data Model

### Items table

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | Auto-generated: `{prefix}-{3 alphanum}`, children: `{parent}.{seq}` |
| `title` | TEXT NOT NULL | |
| `description` | TEXT | |
| `issue_type` | TEXT NOT NULL | task, bug, feature, chore, epic — no special semantics for any type |
| `status` | TEXT NOT NULL | open, in_progress, closed |
| `priority` | INTEGER DEFAULT 2 | 0-4 (0=critical) |
| `parent_id` | TEXT | FK to items.id, nullable |
| `owner` | TEXT | email/name |
| `created_at` | TEXT | RFC3339 |
| `updated_at` | TEXT | RFC3339 |

### Dependencies table

| Column | Type | Notes |
|--------|------|-------|
| `blocked_id` | TEXT NOT NULL | this item is blocked |
| `blocker_id` | TEXT NOT NULL | by this item |
| PRIMARY KEY | (blocked_id, blocker_id) | |

### Relations table

| Column | Type | Notes |
|--------|------|-------|
| `from_id` | TEXT NOT NULL | |
| `to_id` | TEXT NOT NULL | |
| `rel_type` | TEXT NOT NULL | relates_to, duplicates, supersedes |
| PRIMARY KEY | (from_id, to_id) | |

### Notes table

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `item_id` | TEXT NOT NULL | FK to items.id |
| `content` | TEXT NOT NULL | |
| `created_at` | TEXT | RFC3339 |

### Config table

| Column | Type | Notes |
|--------|------|-------|
| `key` | TEXT PK | |
| `value` | TEXT | |

Stores: `prefix` (default: name of directory where `bd init` is run), `owner` (default from git config).

## ID Generation

- Top-level: `{prefix}-{3 random alphanum}` (e.g., `orc-4ho`)
- Children: `{parent_id}.{next_seq}` (e.g., `orc-4ho.1`, `orc-4ho.1.3`)
- Sequence is max existing child seq + 1

## Commands

### Exact surface the orc workflow uses

```
bd init                                    # create .beads/beads.db
bd create --title="" --type= --priority= --parent= -d ""
bd show <id>                               # human-readable detail
bd show <id> --json                        # JSON array: [{id, title, description, status, priority, issue_type, owner, ...}]
bd update <id> --status=                   # change status
bd update <id> --append-notes=""           # add note
bd close <id>                              # set status=closed
bd search "<query>"                        # full-text search title+description
bd dep add <blocked> <blocker>             # add dependency
bd dep relate <a> <b>                      # add relation (relates_to)
bd list --all                              # list everything
bd sync                                    # no-op (print "nothing to sync")
bd ready [parent-id] --json               # ready items, optionally scoped to parent
```

### Additional useful commands

```
bd reopen <id>                             # set status=open
bd delete <id>                             # permanent remove
bd dep remove <blocked> <blocker>          # remove dependency
bd list [--status=] [--type=] [--parent=]  # filtered list
bd docs                                    # print command reference for agents and humans
```

### Display commands (folded in from bdv)

`bdv` was a separate viewer tool. Its features are now built into `bd`:

```
bd                                         # dashboard: epics with children, status counts,
                                           # blocking relationships (replaces bare `bdv`)
bd show <id>                               # detail view: fields, children tree, deps,
                                           # blocked-by, description, notes (replaces `bdv show`)
bd deps                                    # dependency chain DAG across epics (replaces `bdv deps`)
bd ready [parent-id]                       # ready items (replaces `bdv next`)
```

#### Dashboard format (bare `bd`)
```
EPICS                                        status
────────────────────────────────────────────────────────
○ Wave 2: Iteration Speed  9/33 open
  ├── ● 4ho.6    R-033: orc debug — phase execution analysis
  ├── ○ 4ho.16   Stream parser overwrites Usage...
  └── ○ 4ho.27   handleAssistantEvent and handleUserEvent...
  ──▶ blocks: Wave 3: Observability & History
  ◀── blocked by: Wave 1: Convergent Quality
```

- Shows items with `issue_type=epic` as top-level groups
- Lists their open children (hide closed by default, `--all` shows them)
- Shows blocking/blocked-by relationships between epics
- Items with no parent and not epics shown under "ORPHANS"

#### Show format (`bd show <id>`)
```
Wave 2: Iteration Speed
────────────────────────────────────────────────────────
  ID:       4ho  orc-4ho
  Type:     epic
  Status:   ○ open
  Priority: 1
  Owner:    jrbd93@gmail.com
  Created:  Mar 1, 2026

  Children
  ├── ● 4ho.6    R-033: orc debug
  └── ○ 4ho.27   handleAssistantEvent...
  (24 closed children hidden, use --all to show)

  Blocks
  ──▶ ○ Wave 3: Observability & History (9zj)

  Blocked By
  ◀── ✓ Wave 1: Convergent Quality (nuw)

  Description
  <full description text>

  Notes
  <append-only notes if any>
```

#### Deps format (`bd deps`)
```
DEPENDENCY CHAIN
────────────────────────────────────────────────────────
○ 4ho      Wave 2: Iteration Speed  9/33 open
└──▶
  ○ 9zj      Wave 3: Observability & History  4/4 open
  └──▶
    ○ v8i      Wave 4: Client Readiness  4/4 open
```

### `bdv` migration

The orc scripts will be updated: `bdv next` → `bd ready`, `bdv show` → `bd show`, `bdv next --json` → `bd ready --json`. The `bdv` binary is no longer needed.

### JSON Output Formats

**`bd show <id> --json`** — matches current beads format:
```json
[{
  "id": "orc-4ho",
  "title": "Wave 2: Iteration Speed",
  "description": "...",
  "status": "open",
  "priority": 1,
  "issue_type": "epic",
  "owner": "jrbd93@gmail.com",
  "created_at": "2026-03-01T...",
  "updated_at": "2026-03-01T...",
  "dependencies": [...],
  "dependents": [...]
}]
```

**`bd ready --json`** — simplified from beads format:
```json
{
  "total": 9,
  "items": [
    {
      "id": "orc-4ho.6",
      "title": "R-033: orc debug",
      "status": "open",
      "priority": 2,
      "issue_type": "task",
      "parent_id": "orc-4ho"
    }
  ]
}
```

Note: the current scripts parse `jq -r '.epics[0].tasks[0].id'`. We'll update the scripts to parse `.items[0].id` instead when we migrate.

**`bd create` output** — prints the ID:
```
✓ Created issue: orc-4ho.7
  Title: Fix something
  Priority: P2
  Status: open
```

### `ready` Algorithm

An item is ready if ALL of:
1. `status IN ('open', 'in_progress')` — includes WIP items (crash recovery)
2. All blockers (from dependencies table) have `status = 'closed'`
3. If parent-id specified: `parent_id = <parent-id>`

Sorted by priority ASC (0 first), then created_at ASC.

## Architecture

```
~/work/bd/
  cmd/bd/main.go            # CLI entrypoint (urfave/cli/v3)
  internal/db/db.go         # SQLite operations, schema, migrations, queries
  internal/db/db_test.go    #
  internal/model/model.go   # Item, Dependency, Relation, Note structs
  internal/display/show.go  # bd show rendering (detail view)
  internal/display/dash.go  # bd (dashboard), bd deps (DAG view)
  internal/display/list.go  # bd list, bd ready rendering
  go.mod
```

Dependencies:
- `github.com/urfave/cli/v3` — CLI framework (same as orc)
- `modernc.org/sqlite` — pure Go SQLite (no CGo)

### Storage

- Single file: `.beads/beads.db`
- Created by `bd init`
- Located by walking up from cwd (like orc finds `.orc/`)
- Override with `BEADS_DIR` env var

## Migration from existing beads

One-time script: read all items from existing `.beads/beads.db` (if schema-compatible) or export from `bd list --all --json` with old binary, import into new schema. We'll handle this after the core is built.

## What it does NOT do

- No Dolt, no JSONL export, no git sync, no daemon
- No MCP server (future, not now)
- No hooks, no agents command, no prime
- No compaction or summarization
- No special epic semantics — epics are just items with type=epic
- No `bdv` binary — `bd ready` replaces it

## Verification

1. `bd init && bd create --title="test" --type=task` works
2. `bd create --parent=<id>` returns instantly (not hang)
3. All orc scripts work after updating `bdv next` → `bd ready` and JSON path `.epics[0].tasks[0].id` → `.items[0].id`
4. `bd ready <parent-id> --json` returns correct items
5. Round-trip: create → show → update → close → reopen → delete
6. Dependencies: create A, create B, dep add B A, ready shows only A, close A, ready shows B
