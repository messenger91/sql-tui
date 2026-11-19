# TODO.md — Go TUI SQL Client (`sql-tui`)

A Bubble Tea–based terminal client for querying MySQL, PostgreSQL, and SQLite, with a three-pane workflow (connections → query editor → results) plus favorites, export/format toggles, and three advanced features: schema-aware autocomplete, multi-tab async execution, and inline result editing.

---

## 0. Foundations & decisions

- [x] **Confirm Go version** — target `go 1.25` in `go.mod`. (Confirmed: toolchain go1.25.3; `go.mod` declares `go 1.25.0`.)
- [x] **Choose SQLite driver**: `modernc.org/sqlite` (pure Go, no CGO) vs `github.com/mattn/go-sqlite3` (CGO). Prefer `modernc.org/sqlite` for a single-binary TUI. (Chosen: `modernc.org/sqlite` v1.59.0 — newest release whose `go` directive is ≤1.25; v1.60+ requires Go 1.26.)
- [x] **Choose clipboard library**: `github.com/atotto/clipboard` (lightweight) vs `golang.design/x/clipboard` (native, more platforms). Use `atotto/clipboard` as baseline; abstract behind an interface. (Chosen: `atotto/clipboard` v0.1.4, behind an interface in `internal/export`.)
- [x] **Choose result-export approach**: hand-rolled CSV/JSON encoding (stdlib `encoding/csv`, `encoding/json`) vs `github.com/go-data-exporter/exporter`. Hand-rolled keeps deps minimal and gives full control over TUI-specific formatting; evaluate `exporter` if it reduces boilerplate for `sql.Rows` scanning. (Chosen: hand-rolled stdlib.)
- [ ] **Define Bubble Tea architecture**: single root `tea.Model` with a `viewState` enum (`viewWorkspace`, `viewQuery`, `viewResult`) plus overlay states (`favorites`, `exportDialog`, `errorDialog`, `schemaBrowser`). Use `tea.Cmd` for all blocking I/O (DB connect, query execution, file write). **Note**: §11 (multi-tab async) will require moving beyond single-`tea.Cmd` execution to a per-session goroutine + `tea.Program.Send` pattern — design the root model with this in mind from day one (session structs, message routing by session ID).

---

## 1. Project scaffolding

- [x] `go mod init github.com/<you>/sql-tui`. (Module: `github.com/messenger91/sql-tui`.)
- [x] Directory layout:
  ```
  cmd/sql-tui/main.go
  internal/app/          # root model, update loop, view routing, message router
  internal/workspace/    # connection list + connection form
  internal/query/        # SQL editor, autocomplete popup, tab bar
  internal/result/       # table / CSV / JSON rendering, cell editor
  internal/favorites/    # favorite script store
  internal/db/           # driver-agnostic connection + execution + schema introspection
  internal/schema/       # schema cache, table/column model, suggestion index
  internal/export/       # CSV/JSON writers + clipboard helper
  internal/session/      # query session (tab) state, async runner, streaming
  internal/styles/       # Lip Gloss styles
  ```
- [x] Add core deps:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/bubbles` (`list`, `textinput`, `textarea`, `table`, `viewport`, `filepicker`, `help`, `key`, `spinner`)
  - `github.com/charmbracelet/lipgloss`
  - Pinned: bubbletea v1.3.10, bubbles v1.0.0, lipgloss v1.1.0 (v1 API line).
- [x] Add DB drivers behind a registry:
  - MySQL: `github.com/go-sql-driver/mysql`
  - PostgreSQL: `github.com/lib/pq` or `github.com/jackc/pgx/v5/stdlib`
  - SQLite: `modernc.org/sqlite` (or `mattn/go-sqlite3`)
  - Implemented as a `Driver` switch in `internal/db` (no mutable registry); Connect wired for all three, blank-imported drivers.
- [x] Add `github.com/atotto/clipboard`. (Used directly by `internal/export.SystemClipboard`.)
- [x] Add `github.com/sahilm/fuzzy` (or `github.com/lithammer/fuzzysearch/fuzzy`) for autocomplete ranking (§10). (Used by the schema browser filter; §10.3 reuses it for suggestions.)
- [x] Create `Makefile` with `build`, `run`, `test`, `test-race`, `lint`.

---

## 2. Database abstraction layer (`internal/db`)

- [x] Define `Connection` struct: `ID`, `Name`, `Driver` (`mysql` | `postgres` | `sqlite`), `DSN`, optional `Options`, `ReadOnly bool`.
- [x] Define `QueryResult` struct: `Columns []string`, `Rows [][]any`, `RowCount int`, `Elapsed time.Duration`, `Err error`. (`Truncated bool` added for the §6.4 cap.)
- [x] Define `TableMeta` struct (for §10): `Schema`, `Name`, `Kind` (table/view), `Columns []ColumnMeta`; `ColumnMeta`: `Name`, `Type`, `Nullable`, `IsPrimaryKey`, `Ordinal`. (`ForeignKeys []ForeignKey` added for §10.2 FK hints.)
- [x] Implement `Connect(ctx, conn Connection) (*sql.DB, error)` using a driver switch (`sql.Open` + `PingContext`). SQLite is capped at one physical connection so `:memory:` stays consistent.
- [x] Implement `Execute(ctx, db *sql.DB, sql string) QueryResult`:
  - Use `db.QueryContext` for rows; capture `Columns()`.
  - Scan each row into `[]any` via `rows.Scan(ptrs...)`.
  - Handle both result-returning and non-result statements (`ExecContext` fallback when `QueryContext` returns no columns).
  - Guard against unbounded result sets (see §6.4).
  - Implemented as `Execute(ctx, db, sql, maxRows)`: a successful `QueryContext` is never retried as `ExecContext` (that would re-run INSERTs); `ExecContext` is used only when the query reports "no result set".
- [x] Implement `LoadSchema(ctx, db *sql.DB, driver Driver) ([]TableMeta, error)`:
  - MySQL/Postgres: query `information_schema.tables` / `information_schema.columns` / `information_schema.key_column_usage` (scope to the current DB/schema). (MySQL scopes to `DATABASE()`; Postgres to `current_schema()`, with FKs from `pg_constraint` for correct composite mapping.)
  - SQLite: `sqlite_master` for tables/views + `PRAGMA table_info(<t>)` per table; `PRAGMA foreign_key_list(<t>)` for FKs.
  - Return stable, sorted results; tolerate permission errors (partial schema is OK).
  - All three drivers implemented; ordinals normalized to 0-based.
- [x] Add connection lifecycle management: `CloseAll()` on app quit; per-connection `context` cancellation. (Implemented as `db.Manager` with Open/Get/Close/CloseAll; query contexts stay caller-owned.)
- [x] Store DSNs securely? MVP: plaintext in config file under `~/.config/sql-tui/connections.json`; note a future `keyring` option. (Implemented plaintext with 0600 + atomic rename; `os.UserConfigDir()` honors `XDG_CONFIG_HOME`. Keyring remains future work.)
- [x] Placeholder style: expose `Dialect` (`mysql` = `?`, `postgres` = `$n`, `sqlite` = `?`) for §12 DML generation. (`Dialect` also carries identifier quoting: backticks/MySQL, double quotes/Postgres+SQLite.)

---

## 3. Root TUI shell & navigation

- [x] Implement root model in `internal/app`:
  - `Init()` → load connections, focus workspace.
  - `Update()` → route `tea.KeyMsg` to active view; handle global keys (`ctrl+c` quit, `tab` cycle pane, `?` help).
  - `View()` → `lipgloss.JoinHorizontal` / `JoinVertical` for the three-pane layout.
  - Connection loading itself lands with §4; `Init` is a no-op for now.
- [x] Define a `Focus` enum: `focusWorkspace`, `focusQuery`, `focusResult`; render focused pane with a distinct border color.
- [x] **Message router**: root model dispatches custom msgs (`sessionEventMsg{sessionID, event}`) to the owning session/tab — required by §11.
  - Skeleton in place (`sessionEventMsg` routed by `SessionID`); the session lookup is added with §11.1.
- [x] Use `bubbles/help` + `bubbles/key` to auto-generate the footer keybinding bar per active view.
- [x] Handle `tea.WindowSizeMsg` → propagate width/height to child components and recompute layouts.
- [x] **Overlay pattern**: implement a generic modal renderer (centered box, dimmed background) for favorites, export, schema browser, and error dialogs.

---

## 4. Workspace window (connection list)

- [x] Use `bubbles/list` for the connection list:
  - Items implement `list.Item` with `Title()` (connection name) and `Description()` (driver + host/db).
  - Filtering enabled (`/` to filter by name).
- [x] **Add connection** flow:
  - `a` opens a form overlay with `textinput` fields: name, driver (select), DSN.
  - Provide driver-specific DSN hints/placeholders.
  - `enter` validates and calls `db.Connect` via `tea.Cmd`; spinner while connecting.
  - On success, append to list and persist; on failure, show error dialog.
- [x] **Edit / delete** connection: `e` / `d` on selected item; confirm delete.
- [x] On `enter`/`space` on a connection: mark active, move focus to Query window, and **kick off `LoadSchema` asynchronously** (§10). (Schema cache proper arrives in §10.1; the app already stores the result and ignores stale responses.)
- [x] Persist connections to `~/.config/sql-tui/connections.json` (0600 perms); load on startup.

---

## 5. Query window (SQL editor)

- [x] Use `bubbles/textarea` as the editor:
  - Multi-line, vertical scrolling, unicode/paste support.
  - Placeholder: `SELECT * FROM …`.
- [x] Bind **execute** to `ctrl+r` (or `F5`): build a `tea.Cmd` that calls `db.Execute`; emit `queryStartedMsg`, then `queryFinishedMsg(QueryResult)`. *(Superseded by §11 async runner; keep the simple path working first.)* (Editor emits `ExecuteRequestedMsg`; the root model owns execution and replies with `queryFinishedMsg`/`queryFailedMsg`.)
- [x] During execution: show a spinner + "Running…" status in the query pane footer; disable execute key until done.
- [x] On result: set result view state, move focus to Result window, populate table. (Focus + result storage done; table rendering is §6.1.)
- [x] **Editor quality-of-life** (incremental):
  - [ ] Basic SQL keyword highlighting via `textarea` styling (regex-based, optional).
  - [ ] `ctrl+s` opens "Save as favorite" dialog (see §7).
  - [x] History buffer (last N executed queries) with `ctrl+↑/↓` to recall. (Last 50, consecutive duplicates collapsed.)
- [x] **Multi-statement safety**: strip trailing semicolons; if multiple statements detected, MVP executes the first and warns, or split-and-run sequentially with per-statement results (decide). (Decided: string/comment/dollar-quote-aware split, sequential execution, last result shown with statement count.)
- [x] Leave a hook for the autocomplete popup (§10): keep the editor wrapped in a `queryPane` model that owns both the textarea and the popup, so the popup can intercept keys before the textarea. (The popup itself lands in §10.3.)

---

## 6. Response window (results)

### 6.1 Table view
- [x] Use `bubbles/table` for structured results:
  - Columns from `QueryResult.Columns`; rows from `QueryResult.Rows` formatted to strings (handle `NULL` as `NULL`, `time.Time`, `[]byte` → hex/string).
  - Support vertical scrolling and cursor row navigation.
- [x] Render a status line: `N rows · 123ms · table view`.
- [x] **Width handling**: compute column widths from content but cap total width; horizontal scroll via `viewport` if the table exceeds pane width (or use table's built-in horizontal scroll if available). (Widths are shrunk proportionally to fit; no horizontal scrolling needed.)
- [ ] **NULL / type styling**: style `NULL` dim; style numeric/boolean distinctly (optional Lip Gloss). (Values render correctly, but `bubbles/table` cannot style individual cells without injecting ANSI into values; left for §13 polish.)
- [x] **Streaming append hook** (§11): table must support appending rows incrementally from `rowsAppendedMsg` without full re-render flicker. (`result.Model.AppendRows` appends raw + formatted rows and preserves the cursor.)

### 6.2 CSV view
- [x] Render `QueryResult` as CSV text inside a `viewport`.
- [x] Use `encoding/csv` to a `bytes.Buffer`; quote fields correctly. (Uses `strings.Builder`; serialization lives in `internal/export.WriteCSV`.)
- [x] Show header row as first line.

### 6.3 JSON view
- [x] Render as a JSON array of objects (column name → value).
- [x] Use `encoding/json` with `json.MarshalIndent` (2-space indent) for readability. (Uses `json.Encoder` + `SetIndent` through an ordered object so column order is preserved.)
- [x] Handle non-marshalable values (`[]byte`, `time.Time`) by converting to strings before marshal.

### 6.4 Large results
- [ ] Cap in-memory result rows (e.g., 10,000) and display `Showing first N rows` warning.
- [ ] Make the cap configurable in a settings file.
- [ ] Do not render table/CSV/JSON lazily for MVP; note future virtualization if needed. *(§11 streaming reduces peak memory for the table path, but CSV/JSON views still materialize fully — document this.)*

---

## 7. Favorites (named SQL scripts)

- [x] Favorite model: `{ ID, Name, SQL, ConnectionID?, CreatedAt }`.
- [x] Store favorites in `~/.config/sql-tui/favorites.json`.
- [x] **Save flow**: from Query window `ctrl+s` → overlay with `textinput` for name → save current editor content.
- [x] **Browse flow**: `ctrl+f` opens a favorites list overlay (reuse `bubbles/list`); `enter` loads SQL into the query editor.
- [x] **Update/overwrite**: if a favorite with the same name exists, prompt to overwrite.
- [x] **Delete**: `d` in favorites list, confirm.
- [ ] Optional: allow associating a favorite with a connection and auto-select it on load. (Field exists in the model; UI/auto-select deferred.)

---

## 8. Export (CSV / JSON → file / clipboard)

- [x] Add an export dialog overlay triggered by `ctrl+e` in the Response window.
- [x] Dialog options:
  - Format: `CSV` / `JSON`
  - Destination: `File` / `Clipboard`
  - File path (pre-filled `./results-YYYYMMDD-HHMMSS.csv|json`) via `textinput`; `tab` to a `filepicker` for directory selection if desired. (Filepicker skipped; path is typed directly.)
- [x] **File export**:
  - CSV: `encoding/csv` → `os.Create` with 0644; write header + rows; flush and check `writer.Error()`. (`os.OpenFile` with 0644; write/flush/close errors checked.)
  - JSON: `json.NewEncoder(file).Encode([]map[string]any{...})` with indentation. (Ordered-object encoder in `internal/export`.)
  - Report absolute path on success.
- [x] **Clipboard export**:
  - Serialize to string (CSV or JSON) and call `clipboard.WriteAll`.
  - On Linux, `atotto/clipboard` requires `xclip`/`xsel`; surface a clear error if unavailable. (Error shown in an "Export failed" dialog.)
- [x] Show a transient toast/status (`Export complete`) in the footer. (4-second timeout, sequence-guarded.)
- [x] Refactor serialization into `internal/export` so table rendering and export share value-formatting logic.

---

## 9. Toggle response view (table / CSV / JSON)

- [x] Bind `v` (or `ctrl+v`) in the Response window to cycle `Table → CSV → JSON → Table`.
- [x] Maintain current format in root model; re-render the response pane accordingly. (Format state lives in `result.Model`, owned by the root model.)
- [x] Status line reflects current format (e.g., `View: JSON`).
- [x] Preserve scroll position per format? MVP: reset to top on switch; note improvement. (Cursor and scroll reset on every switch.)

---

## 10. **[NEW · high effort]** Schema browser + context-aware SQL autocomplete

**Why it's high effort**: three driver-specific introspection paths, an async cache with invalidation, a popup UI layered over `textarea` that must intercept keys, a tokenizer to know *where* the cursor is in the query, and context rules that pick the right candidate class (tables vs columns vs keywords).

### 10.1 Schema introspection & cache
- [x] `internal/schema` package: `SchemaCache` keyed by `ConnectionID`, storing `[]TableMeta` + a suggestion index (lowercased name → table/column).
- [x] On connection activation, run `db.LoadSchema` in a `tea.Cmd`; show `Loading schema…` in the status bar; non-blocking.
- [x] Refresh triggers: on connect, `ctrl+r` inside the schema browser, and (optional) after a DDL statement is detected in the editor. (On connect plus `refreshSchema` helper; the browser key arrives with §10.2; DDL detection skipped as optional.)
- [x] Handle partial failure (permission denied on some catalogs) — keep what loaded, surface a subtle warning. (Status shows `schema: N objects (partial)` or `schema: unavailable`.)

### 10.2 Schema browser pane/overlay
- [x] `ctrl+b` toggles a schema browser overlay (or a resizable left pane).
- [x] Tree: `schema → table/view → columns (with type + PK marker)`; expand/collapse with `→`/`←`, `enter` to insert the selected identifier at the cursor in the editor.
- [x] Fuzzy filter (`/`) over table and column names.
- [x] Show FK hints inline (e.g., `user_id → users.id`) if `LoadSchema` captured them.

### 10.3 Autocomplete popup
- [x] `internal/query/autocomplete` model: a popup rendered just below the cursor line inside the query pane (use `textarea`'s `Line()`/`Column()` or maintain an independent cursor mirror — decide and document).
  - Decision: the cursor rune index is derived from `textarea.Line()` + `LineInfo()` (`StartColumn + ColumnOffset`). The popup is rendered below the editor viewport (the editor shrinks to make room) rather than overlaid at the cursor row; it sits directly below the cursor for the common single/few-line query.
- [x] Trigger: on every keystroke, if the token under cursor is ≥1 char, compute candidates; debounce ~50ms; also allow manual `ctrl+space`. (Generational 50ms `tea.Tick`, manual `ctrl+space`/`ctrl+@`.)
- [x] **Tokenizer** (lightweight, good enough — not a full SQL parser):
  - [x] Split the text up to the cursor into tokens: identifiers, strings, numbers, punctuation, keywords.
  - [x] Track statement boundaries (`;`) and the current clause by scanning backwards for the nearest clause keyword (`SELECT`, `FROM`, `JOIN`, `WHERE`, `GROUP BY`, `ORDER BY`, `HAVING`, `SET`, `VALUES`, `UPDATE`, `INTO`).
  - [x] Extract `FROM`/`JOIN` table references (including aliases) to scope column suggestions.
- [x] **Context rules**:
  - [x] After `FROM` / `JOIN` / `UPDATE` / `INSERT INTO` → suggest table & view names.
  - [x] After `SELECT` / `WHERE` / `ON` / `GROUP BY` / `ORDER BY` / `HAVING` / `SET` → suggest columns of tables in scope; if no `FROM` yet, suggest all columns across cached tables (grouped by table).
  - [x] After `alias.` → suggest only that table's columns. (Aliases resolve against the full statement, since `FROM` often follows the cursor.)
  - [x] Otherwise → SQL keywords (`SELECT`, `WHERE`, …), plus schema names.
- [x] **Ranking**: exact prefix match > prefix (case-insensitive) > fuzzy (`sahilm/fuzzy`); tie-break by kind (table before keyword) and recency (columns used earlier in the current session rank higher). (Recency is tracked per accepted identifier.)
- [x] **UI**: popup list of ≤10 items, each with icon (`▦` table, `▤` column, `K` keyword), dim type suffix; `↑/↓` to move, `tab`/`enter` to accept, `esc` to dismiss, scroll if >10.
- [x] Acceptance inserts the identifier, replacing the token under the cursor; for columns, insert bare name (not `alias.`) unless the user typed the alias prefix.
- [x] Ensure `esc`/`tab` are only swallowed by the popup when it is open — otherwise pass through to the textarea/pane focus logic.

### 10.4 Tests
- [x] Tokenizer table tests: strings with escaped quotes, comments (`--`, `/* */`), multiple statements, aliases, trailing whitespace.
- [x] Clause/scope resolution tests: multi-table `JOIN` with aliases, subqueries (document: subquery scoping is best-effort). (Subqueries intentionally fall back to all columns.)
- [x] Ranking tests: prefix vs fuzzy ordering, case-insensitivity.
- [x] Golden test for popup `View()`. (`internal/query/testdata/autocomplete_popup*.golden`, regenerated with `go test ./internal/query -run TestGolden -update`.)

---

## 11. **[NEW · high effort]** Multi-tab query sessions with async execution, cancellation, and streaming results

**Why it's high effort**: replaces the single-`tea.Cmd` execution model with a session manager that runs N concurrent queries, routes messages back into the right tab, supports per-tab cancellation, and streams rows incrementally without blocking the UI or leaking goroutines.

### 11.1 Session model
- [x] `internal/session`: `Session` struct holds:
  - `ID`, `ConnectionID`, `Title` (derived from first meaningful token of the query, e.g., `SELECT users …`),
  - editor buffer + cursor, current `QueryResult` (or streaming buffer),
  - `State` (`idle` | `running` | `done` | `cancelled` | `error`),
  - per-format scroll positions, pending cell edits (§12).
  - Implemented with per-session `query.Model`/`result.Model` (buffer, cursor, raw result, scroll); cancel func and timestamps included. Pending-edit state is added with §12.
- [x] `SessionManager` in the root model: `map[SessionID]*Session`, ordered list for tab display, focus tracking.
  - `session.Manager` implemented and tested (`New`/`Get`/`Active`/`SetActive`/`Sessions`/`Close`/`Next`/`Prev`). Root-model wiring lands with the tab bar in §11.2.

### 11.2 Tab bar
- [ ] Tab bar rendered above the query editor: `│ 1: SELECT users … │ 2: ● UPDATE orders … │ +`.
  - `●`/spinner marker when a tab is running.
- [ ] Keys: `ctrl+t` new tab (inherits active connection), `ctrl+w` close tab (confirm if running — offer cancel), `ctrl+tab` / `ctrl+→` next, `ctrl+shift+tab` / `ctrl+←` prev, `alt+1..9` jump to tab.
- [ ] Click on tab (if mouse enabled) to focus.
- [ ] Closing the last tab keeps an empty tab alive (don't leave the query pane blank).

### 11.3 Async runner (core)
- [ ] `internal/session/runner.go`: `Run(ctx, db, sql, onEvent)` opens `rows`, iterates `rows.Next()` in a goroutine, and emits events:
  - `startedMsg{sessionID, columns}`
  - `batchMsg{sessionID, rows [][]any}` — batched every N rows (e.g., 200) **or** every T ms (e.g., 50ms), whichever comes first, to avoid flooding the channel.
  - `doneMsg{sessionID, rowCount, elapsed}`
  - `errorMsg{sessionID, err}`
- [ ] Deliver events via `tea.Program.Send`; the root model routes by `sessionID` to the owning `Session`.
- [ ] **Concurrency**: each session runs at most one query; a second `ctrl+r` in a running session is a no-op (or queues, decide — recommend no-op with a beep/status).
- [ ] **Cancellation**: each session owns a `context.WithCancel`; `ctrl+c`/`esc` while focused on a running session cancels it, emits `cancelledMsg`; the runner goroutine must exit promptly (use `rows.Close()` + select on `ctx.Done()` in the loop).
- [ ] **Goroutine hygiene**: track running runners in a `sync.WaitGroup`; on app quit, cancel all, wait, then `CloseAll()`. No leaked goroutines — assert with `go test -race`.
- [ ] **Streaming into the UI**:
  - [ ] Table view appends rows from `batchMsg`; auto-scroll only if the user is already at the bottom.
  - [ ] Status line shows `Streaming… 4,231 rows · 1.2s` while running, then `4,231 rows · 1.2s · done`.
  - [ ] Respect the §6.4 cap: once the cap is hit, stop appending but keep draining (`rows.Next()` → discard) so the connection isn't left open, and show `Showing first 10,000 rows (truncated)`.
  - [ ] CSV/JSON views render from the accumulated buffer after `doneMsg` (defer rendering while streaming to keep it cheap).
- [ ] **Backpressure**: bounded channel (e.g., cap 8 batches); if the UI is slow, the runner blocks on send — acceptable, but ensure `ctx.Done()` is selected while blocked.

### 11.4 Favorites/export/view-toggle integration
- [ ] Favorites, export, and `v` view toggle operate on the **focused session**; dialog titles include the session/tab name.
- [ ] Exporting a still-streaming session is disabled with a tooltip: `wait for query to finish`.

### 11.5 Tests
- [ ] Session manager unit tests: create/close/switch, title derivation, running-tab close confirmation.
- [ ] Runner tests against SQLite in-memory: batch sizes, cancellation mid-stream (`ctx` cancel after N batches → no goroutine leak, `rows` closed), truncation at cap, error propagation.
- [ ] `go test -race` across the session package; a stress test that runs 10 concurrent queries and asserts all complete/cancel cleanly.
- [ ] Golden tests: tab bar `View()` with mixed states (idle/running/done/error).

---

## 12. **[NEW · high effort]** Inline result editing with generated DML + transactional apply

**Why it's high effort**: needs PK/unique-key detection per driver, type-aware cell editors, per-driver placeholder syntax and quoting, conflict detection on apply, transaction + rollback semantics, and a preview step so the user never silently mutates data.

> **Gate**: entire feature is off unless the connection has `ReadOnly == false` *and* the result set maps to a single table with a detectable PK/unique key. Otherwise the table is read-only and cell-edit keys are inert with a status hint. This promotes the "read-only guard" stretch item to a required part of this feature.

### 12.1 Eligibility detection
- [ ] On query completion, attempt to map the result to a base table:
  - Parse the query (reuse §10.3 tokenizer, shallow) — MVP: only attempt when the query is a simple `SELECT … FROM <table>` with no `JOIN`/`GROUP BY`/aggregates/subqueries; otherwise mark result non-editable.
  - Use cached `TableMeta` (§10.1) to confirm the table exists and identify PK columns (fallback: first `UNIQUE NOT NULL` index; else non-editable).
- [ ] Surface editability in the status line: `Editable · PK: id` vs `Read-only (no PK)` vs `Read-only (complex query)`.

### 12.2 Cell editing UX
- [ ] In the table view, `e`/`enter` on the focused cell opens a `textinput` editor overlay pre-filled with the current value; `ctrl+n` toggles `NULL` (only if the column is nullable).
- [ ] Type-aware validation using `ColumnMeta.Type`: integers, numerics, booleans (`t/f/true/false/1/0`), timestamps (ISO-8601 parse with clear error), text (as-is).
- [ ] Visual diff state: edited cells rendered with a yellow background; `NULL` rendered dim.
- [ ] `esc` cancels the cell edit; `ctrl+z` on a row reverts all pending edits for that row; `ctrl+shift+z` reverts all pending edits in the result.
- [ ] Row-level actions (menu via `m`): `Set cell to NULL`, `Duplicate row` (INSERT), `Delete row` (DELETE) — each becomes a pending change, not an immediate DB write.

### 12.3 DML generation
- [ ] `internal/result/dml.go`: build `Statement{SQL, Args []any}` from pending changes:
  - `UPDATE <table> SET <col> = <ph>, … WHERE <pk> = <ph> AND …`
  - `DELETE FROM <table> WHERE <pk> = <ph> AND …`
  - `INSERT INTO <table> (<cols>) VALUES (<ph>, …)`
- [ ] Placeholders by dialect (§2): `?` for MySQL/SQLite, `$1..$n` for Postgres.
- [ ] Identifier quoting: backticks (MySQL), double quotes (Postgres/SQLite) — escape embedded quote chars by doubling.
- [ ] Value marshalling: `nil` → `NULL`, `time.Time` → dialect-accepted string, `[]byte` → driver handles, booleans → `1/0` for MySQL, `true/false` for Postgres.
- [ ] **Preview overlay** (`p` or automatic before apply): show the generated SQL + args in a `viewport`; `enter` applies, `esc` cancels.
- [ ] Always use placeholders — never string-interpolate user values into SQL (note this explicitly to prevent future regressions).

### 12.4 Transactional apply
- [ ] Apply all pending changes for the focused session in one `db.BeginTx`:
  - [ ] Execute statements in order; on any error → `Rollback`, show error dialog with the failing statement, keep pending changes so the user can fix and retry.
  - [ ] On success → `Commit`; report `N statement(s) applied`.
- [ ] **Conflict detection**: before generating the `UPDATE`/`DELETE`, re-query the row by PK (`SELECT <editable cols> FROM t WHERE pk = ?`) and compare the **old** values to the snapshot taken when the cell was first edited. On mismatch, abort with `Row changed since load — refresh required`; provide `r` to refresh the result (re-run the original query into the session, resetting pending edits after confirmation).
- [ ] Affected-rows sanity check: each `UPDATE`/`DELETE` should report `RowsAffected == 1`; otherwise roll back and report (guards against a mis-detected PK).
- [ ] After commit: refresh the affected cells from the DB in place (re-query by PK) rather than re-running the whole query, unless refresh fails → fall back to re-running.

### 12.5 Tests
- [ ] DML generation table tests per dialect: placeholder numbering, identifier quoting with embedded quotes, `NULL`s, all supported types, `WHERE` with composite PKs.
- [ ] SQLite in-memory integration tests: happy-path update/insert/delete; rollback on mid-batch error; `RowsAffected != 1` abort; conflict detection (mutate a row out-of-band between load and apply).
- [ ] Postgres/MySQL integration tests behind a `//go:build integration` tag using Docker Compose from §14.
- [ ] Ensure pending edits survive a view toggle (`v` table↔CSV↔JSON) and a tab switch.

---

## 13. Styling & polish

- [ ] Define Lip Gloss theme in `internal/styles`: pane borders, focused vs blurred, header, status bar, error text, tab bar (active/inactive/running), pending-edit cell highlight, autocomplete popup.
- [ ] Responsive layout: workspace pane fixed width (e.g., 30 cols), query and result split remaining height; collapse workspace to a tab in narrow terminals (<80 cols) if feasible.
- [ ] Mouse support (optional): enable `tea.WithMouseCellMotion` for list/table/tab scrolling.
- [ ] Error dialog: render `err.Error()` with `viewport` wrapping and `esc` to dismiss.
- [ ] Graceful shutdown: cancel all running sessions (§11), wait, `db.CloseAll()`, save any dirty state.

---

## 14. Testing & validation

- [x] Unit tests for `internal/db`:
  - Use SQLite in-memory (`:memory:`) to test `Execute` with `SELECT`, `INSERT`, empty result, and `NULL` values.
  - Test type formatting (`time.Time`, `[]byte`, `nil`). (Covered in `internal/export` formatting tests.)
  - Test `LoadSchema` against a SQLite fixture with tables, views, PKs, FKs, and a composite PK.
- [x] Unit tests for `internal/export`:
  - CSV round-trip with commas/quotes/newlines. (Newlines are escaped before CSV encoding; quoting is covered.)
  - JSON shape and indentation.
- [ ] Golden tests for `View()` output of each pane state (table/CSV/JSON, empty result), tab bar, autocomplete popup, DML preview.
- [ ] **Cross-cutting integration tests** (`//go:build integration`): Docker Compose for MySQL + Postgres fixtures; exercise the happy path of each advanced feature per driver (schema load, cancellable long query via `SELECT pg_sleep`/`SLEEP`, inline edit apply). (Schema load + query path passes for MySQL/Postgres; long-query cancellation and inline-edit apply arrive with §11/§12.)
- [ ] `go test -race ./...` must pass; goroutine leak check via `go.uber.org/goleak` in session package tests.
- [ ] Manual smoke tests against all three drivers; verify clipboard works on the target platform (or errors clearly).
- [ ] `go vet` + `staticcheck` clean.

---

## 15. Stretch / future

- [ ] Connection via URL (`mysql://user:pass@host/db`) parser.
- [ ] Persistent query history (shared with per-tab recall in §5).
- [ ] EXPLAIN / query-plan visualizer (per-driver plan parsing + tree render).
- [ ] Keyring-backed credential storage.
- [ ] Per-connection default schema/search_path selector (reduces `information_schema` noise from §10).
- [ ] Export to Parquet / Markdown.
- [ ] Virtualized table for >100k-row results (today the §6.4 cap + §11 streaming truncation cover the common case).
- [ ] Column resizing / hide columns in table view (persist per-session).

---

## Dependency summary (initial)

| Purpose | Library |
|---|---|
| TUI runtime | `github.com/charmbracelet/bubbletea` |
| Components | `github.com/charmbracelet/bubbles` (list, textinput, textarea, table, viewport, filepicker, help, key, spinner) |
| Styling | `github.com/charmbracelet/lipgloss` |
| MySQL | `github.com/go-sql-driver/mysql` |
| PostgreSQL | `github.com/lib/pq` (or `github.com/jackc/pgx/v5/stdlib`) |
| SQLite | `modernc.org/sqlite` (preferred) / `github.com/mattn/go-sqlite3` |
| Clipboard | `github.com/atotto/clipboard` |
| Fuzzy matching (§10) | `github.com/sahilm/fuzzy` |
| CSV/JSON | stdlib `encoding/csv`, `encoding/json` |
| Goroutine leak tests (§11) | `go.uber.org/goleak` (test-only) |

---

### Suggested build order for the three new features

1. **§10 first** — it's the most self-contained and pays off immediately in the query window; its tokenizer is reused by §12.
2. **§11 second** — the session manager refactor is invasive; doing it before §12 means §12 builds on the final execution model instead of the temporary `tea.Cmd` path.
3. **§12 last** — depends on §10's `TableMeta`/PK detection and §11's session state for pending edits and refresh-in-place.