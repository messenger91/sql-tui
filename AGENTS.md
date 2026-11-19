# AGENTS.md

## Current state

Completed: all planned sections — bootstrap, `internal/db` (SQLite/MySQL/Postgres connect/execute/schema), root shell, workspace, query editor, result views/export, favorites, schema browser + autocomplete, multi-tab async streaming, inline editing with transactional apply, plus mouse support and narrow-terminal collapse. **M9 and the dependency-free §15 stretch items are complete.** Remaining unchecked items need either a product decision or a new dependency: keyring credential storage, Parquet export, and virtualized >100k tables. `TODO.md` is authoritative for details.

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
- `staticcheck` is installed in `$(go env GOPATH)/bin` (v0.8.2; it downloads a Go 1.26 toolchain for itself). `make lint` finds it even when that directory is not on `PATH`; findings must be fixed, not suppressed.
- Integration tests: `make db-up` starts MySQL (`127.0.0.1:3307`, root/root) and Postgres (`127.0.0.1:5433`, postgres/postgres) via `docker-compose.yml`; `make test-integration` runs `go test -tags integration ./...`; `make db-down` removes them and volumes. Tests cover schema load, cancellable `pg_sleep`/`SLEEP` queries, and inline apply commit/conflict. Override DSNs with `MYSQL_TEST_DSN` / `POSTGRES_TEST_DSN`. Plain `go test ./...` must not require Docker.
- Golden tests use stdlib `testdata/*.golden` comparison; regenerate with `go test ./internal/<pkg> -run TestGolden -update` (`query`, `app`, and `result` each define their own `-update` flag). Sets: autocomplete popup, tab bar + DML preview, and table/CSV/JSON/empty result states.
- `go.uber.org/goleak` runs in `internal/session`'s `TestMain`; session/runner tests must leave no goroutines behind.
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

## Table info / DBML gotchas

- `db.GetTableInfo` is per-driver; the Postgres DDL is *reconstructed* from introspected columns/constraints (not pg_dump), and SQLite/MySQL use real DDL. Views are supported on all three.
- The doc viewer (`app.docViewer`) is a full-width scrollable overlay; it is wired into `resize()` (`resizeDoc`) and into `anyOverlayOpen`. Add new doc producers with `openDoc`, not bespoke modals.
- Workspace keys: `i` works only in the table dropdown (emits `workspace.TableInfoMsg`); `D` emits `workspace.DBMLMsg` from either mode — it is handled in the main switch, NOT the table-mode branch.
- DBML is generated from the cached schema (`schema.Cache`), so it needs a loaded schema; `openDBML` notices when it is missing.

## UI gotchas (post-spec batch)

- The query editor is *rendered* by `query.Model.renderEditor` (highlight + block caret); the textarea only owns input/scroll state. Anything that reads editor output must use `EditorView()`, and `renderLines` clips to `m.width`.
- The workspace has a second `bubbles/list` for tables (`tables`); `SetSize` must size BOTH lists, and `SetTables` is fed from `schemaLoadedMsg`. `t` toggles it, `enter` emits `workspace.FetchTableMsg` (root runs `SELECT * FROM <quoted> LIMIT 100` via the shared `executeSQL` path).
- `x` is the command menu (outside the editor); `E` is explain (results pane). Item run funcs are `func(*Model) tea.Cmd` and `handleCommandMenuKey` returns `m, cmd` after running — never return a `*Model` from those helpers or appUpdate's type assertion breaks.
- Help is a modal (`helpOpen`); `?` opens it outside the editor. `helpText()` is static — update it when bindings change.
- `export.ResultStats` is the single formatter for `rows · speed · elapsed`; the runner tracks drained rows in `Event.Fetched` and `result.Model.UpdateStats` applies them on done.

## Stretch-feature gotchas

- `db.ParseDSN` normalizes URL-style DSNs before `sql.Open`; `Connection.DSN` stays as entered for display, so `maskDSN` still sees URLs.
- `Connection.Options["schema"]` is applied after ping (`SET search_path` / `USE`); SQLite rejects it. The connection form exposes it as the optional `Schema` field.
- `history.json` is loaded asynchronously on `Init` and seeded into every session editor; `recordHistory` only runs when a query actually starts. New tabs are seeded explicitly in `focusSessionForConnection`/`newTab`/`closeSession`.
- `x` explains the first statement via `db.Explain`; SQLite trees are built from `id/parent/detail`, everything else renders text rows. Plans open in the generic modal and are height-clipped.
- Markdown export lives in `internal/export/markdown.go` and is part of `Serialize`/`WriteFile`; the dialog cycle is CSV → JSON → Markdown.
- Table layout keys live in `result.Model.Update`: `H` toggles hide, `[`/`]` resize, `0` resets. Hidden/resized columns persist by lowercased column name across view switches and matching results. Rebuild clears rows before changing columns to avoid a bubbles/table panic.
- Adding keyring or Parquet support requires new dependencies — do not add them without explicit approval.

## DML / preview gotchas

- `session.Changes()` is the bridge from `Pending` to `result.Change`; it requires editable state, the loaded result, and `Session.Driver` (copied from the connection like `ReadOnly`).
- Row order drives statement order; delete rows take precedence over cell edits, duplicates include every result column with pending edits overlaid, and no-op edits are skipped.
- `result.BuildStatements` never interpolates values; identifiers go through `db.Dialect.QuoteIdent`. MySQL booleans become `1/0`; time/bytes pass through to `database/sql`.
- The `p` preview lists SQL + args and is gated on pending changes and the table view. `enter` runs `startApply`; there is no other apply path.
- Apply snapshots the loaded row on the first edit (`snapshotRow`); `Session.ApplyPlan` re-reads rows inside the transaction and aborts with `ErrRowChanged` on drift. Pending edits survive apply failures and conflicts.
- `UPDATE`/`DELETE` must affect exactly one row; otherwise the transaction rolls back with `ErrRowsAffected`.
- On success the app clears pending, reports `N statement(s) applied`, and refreshes the view **in place**: the apply command also runs `ApplyPlan.RefreshMutations` (re-reads affected rows by PK, handling PK edits) and returns `RowMutation`s that the handler replays on `result.Model` via `ReplaceRowValues`/`RemoveRowResult`/`AppendRows` (removes sorted descending). Refresh failures fall back to the query re-run.
- Conflict modal instructs the user to close it and press `r` in the results pane; `r` clears pending and re-runs `Session.LastSQL`.
- Do not build SQL strings anywhere else; extend `result/dml.go` instead.

## Cell editing gotchas

- Connections are **read-only by default**; the add/edit form has a fourth `Access` field (tab to it, `←/→` toggles writable). Writable is required for §12 eligibility.
- Pending edits live on `Session.Pending` (row index → column → `CellEdit` + row ops). `App.syncPending` copies row markers/count into the active session's `result.Model`; call it after every mutation.
- Result cell selection is a separate column cursor (`cellCol`, moved with `←/→`/`h`/`l`); rows still use the table cursor. The table cannot color cells, so pending rows are prefixed with `* ` and the status shows `N pending`.
- `e`/`enter` opens the cell editor only in the table view for editable results; read-only results show a `result is read-only` notice.
- Validation lives in `session.ParseCellValue`; NULL is set with `ctrl+n` (nullable columns only). `esc` cancels; `ctrl+z` reverts the selected row; `ctrl+shift+z` reverts all (terminal support for that chord varies).
- A new query clears pending edits and resets editability.
- After streaming, the table cursor is at the last row (bottom-following); tests must read `SelectedCell()` instead of assuming row 0.

## Inline editing gotchas

- Eligibility is computed on `EventDone` from `Session.LastSQL`, the result columns, the cached schema index, and `Session.ReadOnly`. Results without a result set keep an empty `Editability`.
- `Session.ReadOnly` is copied from the connection when a tab is bound/reused; other sessions are unaffected. A read-only connection reports `read-only (connection read-only)`.
- `query.SimpleSelectTable` is shallow: rejects `(`, joins, `GROUP BY`/`HAVING`, `UNION`/`INTERSECT`/`EXCEPT`, `DISTINCT`, multiple statements, multiple FROM tables; allows WHERE/ORDER/LIMIT/OFFSET and aliases.
- PK-only eligibility is deliberate (approved 2026): no unique-index fallback yet. Result columns must map exactly by name; aliases and duplicate columns are read-only.
- Status reason strings come from `session.ComputeEditability`; keep them short or the footer truncates.

## Async runner gotchas

- Favorites, export, and `v` all read the **active session**; favorites dialog titles get ` · <tab title>` via `favoritesDialog()`, export uses `Export results · <tab title>`.
- Export is refused while the active session is running (`wait for the query to finish` notice) instead of opening a dialog with partial data.
- `main` must call `model.SetSender(program.Send)` **before** `program.Run()`; the sender holder is a pointer shared with the program's model copy.
- Each run spawns exactly two goroutines (runner + `Program.Send` forwarder) counted by `runnerGroup`. Shutdown clears the sender **before** cancelling/waiting so a forwarder can never block inside `Send` after the event loop stops.
- Batch/started events select on `ctx.Done()`; terminal events (`done`/`error`/`cancelled`) are sent unconditionally so the UI reliably learns the outcome.
- `ctrl+c` cancels the active running session first; a second `ctrl+c` runs graceful shutdown (`shutdownDoneMsg` → quit). `esc` cancels a running session when no modal/popup is open.
- `EventDone` switches focus to the results pane when the session is active (preserves the §5 behavior). Streaming batches update `result.Model` in place; CSV/JSON serialization is deferred until streaming stops.
- The runner drains rows past the 10k cap (never leaves the connection mid-result) and reports `Truncated` in the final event.

## Tab / session wiring gotchas

- Root state is the `session.Manager`; use `m.activeSession()` and never add global query/result fields. Favorites, export, schema, autocomplete, and view toggles all act on the active session.
- Activation reuses the initial empty tab, then reuses any tab for the same connection; a different connection creates a new tab. Closing the last tab creates a replacement bound to the current connection.
- The tab bar consumes one row of the query pane: sessions are sized with `queryH-4` (border/title + tab row).
- `ctrl+left/right` are global tab navigation (decision d3), so they shadow textarea word movement. `alt+1..9` jump by position; out-of-range digits are ignored.
- `ctrl+w` on a running tab sets `confirmClose` and renders a confirmation modal; `y` closes (manager cancels), `n`/`esc` cancels. This modal is checked before regular key routing.
- Session titles update on execution (`startQuery`), not while typing.

## Session gotchas

- `session.Session` owns a **per-session** `query.Model` and `result.Model`; there is no shared editor buffer. Do not add global query/result state to the root model — read from the active session.
- `session.Manager` is not internally locked: mutate it only from the root model's update loop. Runners communicate by messages and must never touch it directly.
- `Close` cancels a running session and picks the neighbouring tab as active; the caller must create a replacement tab when the last one closes (§11.2).
- Titles come from `DeriveTitle`: leading `--`/`/* */` comments skipped, whitespace collapsed, 23 runes + `…` max.

## Autocomplete gotchas

- Cursor position in runes: `textarea.Line()` + `LineInfo().StartColumn + ColumnOffset` (there is no exported column/cursor getter).
- The 50ms debounce is generational (`suggestGen`) and validates both generation and current editor text. `scheduleSuggest` must be a **pointer** receiver and its command must be built before returning `m`, otherwise the incremented generation is lost.
- Context clause uses the text **before** the token; alias/table refs for `alias.` completion use the **whole statement**, because `FROM` often follows the cursor.
- The popup is drawn below the editor viewport (editor height shrinks), not overlaid at the cursor row; acceptable for short queries, a known approximation.
- Acceptance simulates right-arrow/backspace key presses to replace the token span, preserving the textarea cursor; it never calls `SetValue` (which would jump to the end).
- The root model intercepts `tab` and plain runes only while `query.PopupVisible()`; otherwise global bindings (`tab` cycle, `?` help) apply.
- Ranking uses `sahilm/fuzzy` for the fuzzy tier; exact and prefix tiers are computed locally, then kind rank and accepted-identifier usage break ties.

## Schema browser gotchas

- `ctrl+b` needs an active connection **and** a completed schema load; otherwise it emits a notice (`select a connection first`, `schema still loading`, `schema not loaded`).
- The browser reads a cache snapshot on open and is updated by `schemaLoadedMsg` while open, so `ctrl+r` inside it refreshes in place (the root model's `handleBrowserKey` calls `refreshSchema`).
- Fuzzy filtering switches the tree to a flat, score-ordered list of matching tables/columns; columns show their owning table in parentheses.
- `enter` inserts a **bare identifier** (column name without table/alias prefix), then closes the overlay and focuses the query pane.
- The modal is rendered after workspace/export/favorites modals and before the generic app modal; keys are routed to it only while it exists.

## Schema cache gotchas

- `schema.Cache` is concurrency-safe and stores an immutable `*Index` snapshot per connection; read it with `Get`, never mutate an entry's slices (they are shared).
- The index is case-insensitive and keeps duplicate names (same column in several tables); `Find` returns all matches and each column suggestion carries its owning table.
- Activation marks `schemaLoad` and calls `refreshSchema`; the status bar renders `schema: loading…`, `schema: N objects`, `schema: N objects (partial)`, or `schema: unavailable`.
- Deactivation clears the active connection but **keeps** cached schemas; activation always refreshes. Deleting a connection does not evict its cache entry yet.
- `ctrl+r` inside the schema browser (§10.2) must call `refreshSchema`; the query pane's `ctrl+r` still executes SQL because keys are routed by focus.

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
- The workspace pane collapses progressively below 80 columns and is hidden below 25 (`wsHidden`); `tab` cycling and mouse handling must respect it (`skipHiddenWorkspace`, `handleMouse`).
- Mouse: `main` enables `tea.WithMouseCellMotion`. `handleMouse` only acts on left-button presses and ignores clicks while any overlay is open; tab-bar clicks map through plain `tabLabels()` widths (styles must not change display width).
- `?` toggles help only outside the query pane; inside the editor it is an ordinary character (SQL placeholders).
- The workspace list marks the active connection with `●` and its description notes `read-only`; empty panes carry key hints (connections: `a add · e edit · d delete · enter connect`; results: `ctrl+r` hint).
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
