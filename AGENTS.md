# AGENTS.md

## Current state

Completed: §0–§9 — bootstrap, `internal/db` (SQLite, MySQL, and Postgres: connect, execute, schema), root TUI shell, workspace pane, query editor, table/CSV/JSON result views, view toggle, and export to file or clipboard. **M3, M4, and M5 are complete.** Next: §10 schema browser + autocomplete (M6). `TODO.md` is authoritative for what is implemented.

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
- Integration tests: `make db-up` starts MySQL (`127.0.0.1:3307`, root/root) and Postgres (`127.0.0.1:5433`, postgres/postgres) via `docker-compose.yml`; `make test-integration` runs `go test -tags integration ./...`; `make db-down` removes them and volumes. Override DSNs with `MYSQL_TEST_DSN` / `POSTGRES_TEST_DSN`. Plain `go test ./...` must not require Docker.
- Manual PTY smoke tests: run under `TERM=dumb` (e.g. `script -qec "./bin/sql-tui" /dev/null`). With a colour-capable `TERM`, termenv blocks waiting for an OSC background-colour reply that `script` never sends, so the app appears to hang before entering raw mode.

## Architecture invariants

These are easy to violate when writing the first implementation:

- **Blocking I/O never runs in `Update`/`View`.** DB connects, queries, file writes, and clipboard access go through `tea.Cmd` or async runners.
- **Design for §11 from day one.** Even the simple single-tab path must route messages by `sessionID` (`sessionEventMsg`) so the later per-session goroutine + `tea.Program.Send` model drops in without rewriting the root model.
- **Always use SQL placeholders for user values** — `?` for MySQL/SQLite, `$n` for Postgres. Never interpolate values into SQL (§12.3 calls this out explicitly to prevent regressions). Identifier quoting differs per dialect: backticks for MySQL, double quotes for Postgres/SQLite.
- **Inline editing is gated.** It is enabled only when the connection has `ReadOnly == false` *and* the result maps to a single table with a detectable PK/unique key. Pending edits are applied in one transaction with rollback and `RowsAffected == 1` checks; never write immediately on a cell edit.
- **Goroutine hygiene is testable.** Cancellation must use `context`, runners must be tracked and drained on quit (`CloseAll`) and covered by `go test -race` plus `go.uber.org/goleak` in the session package.
- **Drivers are a registry, not ad-hoc imports.** `internal/db` owns a driver switch/registry exposing `Connect`, `Execute`, `LoadSchema`, and a `Dialect` value used by §12 DML generation.

## Workspace gotchas

- `connections.json` is `{"connections":[...]}` written atomically with 0600. A missing `readOnly` field loads as `true`; keep that default when touching the store.
- The workspace owns its own dialogs (add/edit form, delete confirm, connect error). It exposes them via `Modal()` and reports `CapturesInput()`, which the root model checks **before** global `tab`/help handling. Add new modal keys there, not in `app`.
- Activation is message-based: workspace emits `ActivatedMsg`; the root model focuses the query pane, calls `workspace.ActivationSucceeded` (which also clears the `connecting…` spinner), and starts an async `db.LoadSchema`. Editing the active connection re-emits `ActivatedMsg` so the schema is reloaded. If a new activation message is ever added, clear `busy` there too or the spinner runs forever.
- DSNs are masked in the list (`maskDSN`), but the form and the on-disk JSON keep them plaintext.
- `main` must call `model.Close()` after `tea.NewProgram(...).Run()`; the manager is a pointer inside a copied model, so closing there releases all handles.

## Favorites gotchas

- `favorites.json` is `{"favorites":[...]}` written atomically with 0600, mirroring `connections.json`. `Config.FavoritesPath` is empty in zero-value configs; the store then keeps data in memory.
- `ctrl+s`/`ctrl+f` only act when the **query** pane is focused. Loading emits `favorites.LoadMsg`, which the root model turns into `query.SetSQL` plus a notice.
- The save form stays alive under the overwrite prompt: after `n` the user returns to the form. Modal/handle ordering must therefore check `overwrite` and `confirm` **before** `save` (both in `Modal()` and `handleKey`).
- While the favorites list is filtering, all keys are delegated to `bubbles/list`, so `d` types into the filter rather than deleting.
- Shortcut `case` branches in `app.handleKey` must include their focus guard **in the case condition** (e.g. `key.Matches(...) && m.focus == FocusResult`). A guard inside the branch returns early and swallows ordinary characters such as `v` while typing in the editor.

## Export gotchas

- `ctrl+e` only opens the dialog when the results pane is focused and a result exists; otherwise it sets a transient notice.
- The dialog lives in `internal/app/export_dialog.go`; the root model performs the write in a `tea.Cmd` (never in `Update`). Clipboard access goes through `export.Clipboard`, injectable via `Config.Clipboard` (tests use a fake).
- `export.WriteFile` opens with 0644, writes via `WriteCSV`/`WriteJSON`, checks write/close errors, and returns the absolute path. `export.Serialize` is the clipboard path.
- Footer notices are sequence-guarded (`noticeSeq`) so a stale 4-second timer cannot clear a newer message.
- `NULL` is exported as the literal `NULL` (shared with the table view). If that changes, change only `export.WriteCSV`/`FormatValue`.

## Result gotchas

- `result.Model` owns the raw `db.QueryResult` for export (§8) and the rendered table. Read it through `Result()`; do not duplicate result state in the root model.
- `internal/export` owns value formatting (`FormatValue`, `ValueRows`) and serialization (`WriteCSV`, `CSVString`); result views and the §8 exporter must use it rather than reimplementing formatting. `NULL` serializes to the literal `NULL`.
- `v` cycles the result view only when the results pane is focused; `CycleFormat` resets table cursor and viewport offset on every switch (MVP behavior).
- CSV/JSON views render through a `bubbles/viewport` and are switched with `SetFormat`/`CycleFormat`. Long lines are clipped by the viewport (no horizontal scroll).
- JSON uses an ordered-object marshaler (`internal/export/json.go`) because `map[string]any` would sort keys. Numbers/bools stay JSON-native; blobs and times become strings; duplicate column names are preserved.
- Values are stringified once by `FormatValue`: `NULL`, RFC3339 time, `0x…` for binary blobs, printable `[]byte` as text, and newlines/tabs escaped so rows stay one line (this also keeps CSV records single-line).
- Column widths are natural but capped (3–40) and shrunk to fit the pane; cells are ellipsized by `bubbles/table`. There is no horizontal scrolling, by design.
- `SetResult` must rebuild the table **before** `SetCursor(0)`: the table clamps the cursor against the current row count, so setting it first leaves the cursor at -1.
- `bubbles/table` applies styles per row, not per cell. Proper NULL dimming would require ANSI inside cell values (which breaks width math) — deferred to §13.
- `AppendRows` keeps the cursor and updates `RowCount`; use it for §11 streaming instead of calling `SetResult` per batch.

## Query gotchas

- The editor never touches the database. `ctrl+r`/`F5` emits `query.ExecuteRequestedMsg`; the root model runs `SplitStatements` and `db.Execute` for each statement, then sends `queryFinishedMsg` or `queryFailedMsg`. Keep execution out of `internal/query` so §11 can replace it with sessions.
- `SplitStatements` is the shared lightweight scanner: it understands quotes, doubled quotes, backslash escapes, `--`/`/* */` comments, and `$$`/`$tag$` blocks. §10/§12 must reuse or extend it rather than adding a second parser.
- Multi-statement runs are sequential and only the **last** result set is shown; the count is reported in the query status. An error aborts the batch with `statement N:` context.
- `MarkRunning` exists because `ExecuteRequestedMsg` can be injected directly; it guarantees the second `ctrl+r` is ignored even if the request did not originate from the textarea.
- History is capped at 50 entries, collapses consecutive duplicates, and `ctrl+↑`/`ctrl+↓` navigate it. The draft buffer restores when moving past the newest entry.

## App shell gotchas

- Layout is computed from `WindowSizeMsg`; before the first resize it falls back to 80×24. `layout()` derives help height from the rendered help view, so expanding `?` full help shrinks the pane area — do not hardcode a one-line footer.
- A pane's content includes its title row: children must be sized to `outerHeight-3` (border + title), or the pane grows a line and the full-screen layout drifts. Width is `outerWidth-4` (border + horizontal padding).
- The modal compositor (`overlay.go`) uses `github.com/charmbracelet/x/ansi` (already a transitive dep of Lip Gloss) for cell-accurate splicing. It replaces the horizontal span and keeps background margins intact; tests cover ANSI-styled backgrounds.
- While a modal is open it swallows all keys except `ctrl+c` (quit). `esc` closes it.
- Focus cycles `workspace → query → results` with `tab`; `shift+tab` reverses.

## `internal/db` gotchas

- `Execute(ctx, handle, query, maxRows)` takes the row cap explicitly; `maxRows <= 0` means `DefaultMaxRows`. A successful `QueryContext` is **never** retried as `ExecContext` — retrying would double-run `INSERT`s. `ExecContext` is only used when the driver reports "no result set".
- SQLite handles are capped at **one physical connection** so `:memory:` has one consistent database. Never run a query while still iterating another `*sql.Rows` on the same handle; `LoadSchema` deliberately closes the `sqlite_master` listing before issuing PRAGMAs.
- `Connection{}` has `ReadOnly: false`; use `db.NewConnection(...)` when constructing a user connection so the read-only default applies. Persistence code must not unmarshal into a fresh `Connection` and lose that default.
- `LoadSchema` returns partial results plus a joined error when only some objects fail. MySQL scopes to `DATABASE()`, Postgres to `current_schema()`; ordinals are normalized to 0-based. MySQL returns `[]byte` for VARCHAR values — `export.FormatValue` handles that, and tests normalizing driver values must too.

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
