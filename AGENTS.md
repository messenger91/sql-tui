# AGENTS.md

## Current state

Bootstrap (§0–§1) is complete: module, directory skeleton, `Makefile`, and a placeholder `cmd/sql-tui/main.go` exist. There is no `internal/...` code or test suite yet, so `go test ./...` currently reports "no test files" — don't treat that as verification of anything. `internal/app` will replace the placeholder model in §3. `TODO.md` is the authoritative plan; check it for what is actually implemented.

## Source of truth

`TODO.md` is the authoritative plan, ordered by section (§1–§15). Read the relevant section before implementing a feature; the section text contains decisions and constraints that are not repeated here. `TODO.md` also specifies the intended build order for the three large features: §10 (schema/autocomplete) → §11 (multi-tab async sessions) → §12 (inline editing), because §12 depends on both.

## Toolchain and module

- Target Go 1.25 in `go.mod` (declared `go 1.25.0`; toolchain is go1.25.3).
- Module path: `github.com/messenger91/sql-tui` (matches `origin`); the layout uses `cmd/sql-tui/main.go` plus `internal/...`.
- Charm stack is the **v1 API line**: bubbletea v1.3.10, bubbles v1.0.0, lipgloss v1.1.0. Do not bump to v2 (`/v2` module paths change `tea.Model.View` and key-message types).
- Prefer `modernc.org/sqlite` (pure Go, no CGO) so the TUI stays a single binary. Pin **v1.59.0**: v1.60+ declares `go 1.26` and would force a toolchain download on Go 1.25.

## Commands

| Command | Meaning |
|---|---|
| `make build` | binary to `bin/sql-tui` |
| `make run` | `go run ./cmd/sql-tui` |
| `make test` / `make test-race` | unit tests (plain / race) |
| `make lint` | `gofmt -l` check, `go vet`, `staticcheck` if installed |
| `make fmt` | `gofmt -w .` |

- Single test: `go test -race -run TestName ./internal/db/`.
- `staticcheck` and `golangci-lint` are not installed in this environment; `make lint` skips `staticcheck` with a notice.
- Integration tests for MySQL/Postgres use the `//go:build integration` tag and require Docker Compose fixtures; plain `go test ./...` must not require them.

## Architecture invariants

These are easy to violate when writing the first implementation:

- **Blocking I/O never runs in `Update`/`View`.** DB connects, queries, file writes, and clipboard access go through `tea.Cmd` or async runners.
- **Design for §11 from day one.** Even the simple single-tab path must route messages by `sessionID` (`sessionEventMsg`) so the later per-session goroutine + `tea.Program.Send` model drops in without rewriting the root model.
- **Always use SQL placeholders for user values** — `?` for MySQL/SQLite, `$n` for Postgres. Never interpolate values into SQL (§12.3 calls this out explicitly to prevent regressions). Identifier quoting differs per dialect: backticks for MySQL, double quotes for Postgres/SQLite.
- **Inline editing is gated.** It is enabled only when the connection has `ReadOnly == false` *and* the result maps to a single table with a detectable PK/unique key. Pending edits are applied in one transaction with rollback and `RowsAffected == 1` checks; never write immediately on a cell edit.
- **Goroutine hygiene is testable.** Cancellation must use `context`, runners must be tracked and drained on quit (`CloseAll`) and covered by `go test -race` plus `go.uber.org/goleak` in the session package.
- **Drivers are a registry, not ad-hoc imports.** `internal/db` owns a driver switch/registry exposing `Connect`, `Execute`, `LoadSchema`, and a `Dialect` value used by §12 DML generation.

## Decided behavior (approved during planning)

- `ctrl+c`: cancels the focused session's running query; quits only when nothing is running. A second `ctrl+c` always quits.
- Multi-statement input: split on `;` with a string/comment-aware scanner and run statements sequentially; show the last result set plus an `N statements executed` status.
- Tab navigation: `ctrl+right` / `ctrl+left` plus `alt+1..9` are primary. `ctrl+tab` is only accepted when the terminal actually reports it.
- Settings: `~/.config/sql-tui/settings.json`, key `max_rows` (default 10000).
- Config paths go through `os.UserConfigDir()` (honors `XDG_CONFIG_HOME`) with `~/.config/sql-tui` fallback, so tests can redirect with `t.Setenv`.
- Golden tests: stdlib `testdata/*.golden` comparison with an `-update` flag; no golden library dependency.
- `db.Execute`: call `QueryContext`; fall back to `ExecContext` on `driver.ErrSkip` or a driver "no result set" error; empty column sets count as exec results.

## Conventions

- State/config files live under `~/.config/sql-tui/` (`connections.json` at 0600, `favorites.json`); DSNs are plaintext for now.
- Per §6.4, cap in-memory result rows (10,000 is the proposed default, configurable) and surface truncation in the status line instead of rendering unbounded results.
- Formatting/export logic lives in `internal/export` and is shared with the table renderer; don't duplicate value formatting between views and export.
- Pane/component models own their own state and are composed by `internal/app`; the root model only routes keys/messages and lays out panes.
