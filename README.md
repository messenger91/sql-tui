# sql-tui

`sql-tui` is a terminal SQL client built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
It connects to **SQLite**, **MySQL**, and **PostgreSQL** and gives you a
three-pane workflow — connections, query editor, results — plus favorites,
CSV/JSON export, and three bigger features: schema-aware autocomplete,
multi-tab async streaming, and inline result editing with a transactional
apply.

```
╭─ Connections ─┬─ Query ────────────────╮
│ ● local       │ 1: SELECT * FROM users │
│   staging     ├────────────────────────┤
│               │ Results                │
│               │ id  email              │
╰───────────────┴────────────────────────╯
 sql-tui · focus: query · conn: local · schema: 3 objects · 120×30
```

## Features

- **Connections** for SQLite/MySQL/Postgres, persisted to `connections.json`
  (0600, atomic writes). Connections are read-only by default; flip the
  `Access` field to `writable` in the add/edit form to enable inline editing.
- **Query editor** with 50-entry history (`ctrl+↑/↓`), multi-statement
  splitting (string/comment/dollar-quote aware), and a 50 ms-debounced,
  schema-aware autocomplete popup (`ctrl+space`).
- **Schema browser** (`ctrl+b`): expandable table/view/column tree with PK and
  FK hints, fuzzy filter, and identifier insertion at the cursor.
- **Results**: table / CSV / JSON views (`v`), capped at 10,000 rows with
  truncation markers, export to file or clipboard (`ctrl+e`).
- **Favorites**: named SQL snippets persisted to `favorites.json`
  (`ctrl+s` to save, `ctrl+f` to browse).
- **Multi-tab sessions**: one editor + result view per tab, executed
  asynchronously with streaming batches, cancellation (`esc`/`ctrl+c`), and
  goroutine-leak-checked shutdown.
- **Inline editing** on writable connections: edit cells (`e`), set NULL
  (`ctrl+n`), duplicate/delete rows (`m`), review generated SQL (`p`), and
  apply everything in one transaction with conflict detection.
- **Mouse support**: click panes/tabs/`+` to focus, wheel to scroll.

## Install

Requires Go 1.25+.

```sh
go install github.com/messenger91/sql-tui@latest
# or
git clone https://github.com/messenger91/sql-tui && cd sql-tui
make build   # -> bin/sql-tui
```

## Quick start

1. Run `sql-tui`.
2. Press `a` in the Connections pane, pick a driver, enter a DSN, and choose
   the `Access` field (`read-only`/`writable`) with `←/→`.
3. Press `enter` to connect. The schema loads in the background.
4. Type SQL in the query pane and run it with `ctrl+r` (or `F5`).
5. Browse results with the arrow keys, press `v` to switch table/CSV/JSON, and
   `ctrl+e` to export.

### Connection DSN examples

| Driver  | DSN |
|---|---|
| SQLite  | `:memory:` or `/path/to/file.db` |
| MySQL   | `user:pass@tcp(127.0.0.1:3306)/dbname` |
| Postgres| `postgres://user:pass@localhost:5432/dbname?sslmode=disable` |

DSNs are stored in plaintext locally; passwords are masked in the list UI.

## Keybindings

| Key | Action |
|---|---|
| `tab` / `shift+tab` | cycle pane focus |
| `ctrl+t` / `ctrl+w` | new tab / close tab (confirms when running) |
| `ctrl+→` / `ctrl+←`, `alt+1..9` | switch/jump tabs |
| `ctrl+r`, `F5` | execute SQL |
| `ctrl+space` | open autocomplete |
| `ctrl+↑` / `ctrl+↓` | query history |
| `ctrl+b` | schema browser (`ctrl+r` inside refreshes) |
| `ctrl+s` / `ctrl+f` | save / browse favorites |
| `v` | cycle table → CSV → JSON (results pane) |
| `ctrl+e` | export CSV/JSON to file or clipboard (results pane) |
| `e`, `ctrl+n`, `m` | edit cell, set NULL, row menu (results pane) |
| `p` | preview generated SQL, `enter` applies transactionally |
| `ctrl+z` / `ctrl+shift+z` | revert row / all pending edits |
| `esc` | cancel running query, close overlay |
| `ctrl+c` | cancel the running query, then quit |
| `?` | toggle help (outside the editor) |
| `a` / `e` / `d` / `/` | add / edit / delete / filter connections |

Mouse: click a pane to focus it, click a tab (or `+`) in the tab bar, scroll
lists/tables with the wheel.

## Data & settings

State lives under your config directory (honours `XDG_CONFIG_HOME`):

- `sql-tui/connections.json` — connections, 0600, atomic writes
- `sql-tui/favorites.json` — favorite scripts, 0600

Result sets are capped at 10,000 in-memory rows (truncation is shown in the
status line). Favorite/connection dialogs always ask before overwriting.

## Development

```sh
make build          # bin/sql-tui
make run            # go run ./cmd/sql-tui
make test           # unit tests (no external services)
make test-race      # unit tests with -race
make lint           # gofmt + vet + staticcheck
make db-up          # start MySQL/Postgres fixtures (3307/5433)
make test-integration  # go test -tags integration ./...
make db-down        # remove fixtures
```

- Golden tests live in `internal/{query,app,result}/testdata`; regenerate with
  `go test ./internal/<pkg> -run TestGolden -update`.
- `internal/session` runs `goleak` in its `TestMain`.
- `staticcheck` is expected in `$(go env GOPATH)/bin`.

Project layout:

```
cmd/sql-tui/        entrypoint
internal/app/       root model, layout, modals, mouse, export dialog
internal/db/        driver-agnostic connect/execute/stream/introspection
internal/query/     editor, statement splitter, autocomplete
internal/result/    table/CSV/JSON rendering, DML generation
internal/schema/    per-connection schema cache + suggestion index
internal/session/   tabs, async runner, pending edits, apply
internal/workspace/ connection list, forms, persistence
internal/export/    value formatting, CSV/JSON writers, clipboard
internal/favorites/ favorite store + browse overlay
internal/styles/    Lip Gloss theme
```

`AGENTS.md` documents the working conventions and gotchas for agents;
`TODO.md` is the authoritative spec with per-section checkboxes and notes.

## Limitations

- Editing requires a writable connection and a simple `SELECT … FROM <table>`
  with a primary key present in the result.
- Pending edits are applied transactionally; the post-commit view refreshes by
  re-running the original query.
- Subquery-aware autocomplete scoping and per-cell coloring are best-effort.
