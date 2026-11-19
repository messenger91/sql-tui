package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/export"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/styles"
	"github.com/messenger91/sql-tui/internal/workspace"
)

const (
	defaultWidth      = 80
	defaultHeight     = 24
	workspaceWidth    = 30
	minWorkspaceWidth = 12
	minBodyHeight     = 3
	schemaTimeout     = 10 * time.Second
)

type modalBox struct {
	Title string
	Body  string
}

// Config carries filesystem locations and injectable services. The zero
// value keeps persistence in memory and uses the system clipboard.
type Config struct {
	ConnectionsPath string
	Clipboard       export.Clipboard
}

// Model is the root application model.
type Model struct {
	theme     styles.Theme
	help      help.Model
	keys      keyMap
	manager   *db.Manager
	workspace workspace.Model
	query     query.Model
	results   result.Model
	clipboard export.Clipboard

	focus         Focus
	width, height int
	modal         *modalBox
	export        *exportDialog
	notice        string
	noticeSeq     int

	active       *db.Connection
	schema       []db.TableMeta
	schemaErr    error
	lastQueryErr error
}

// New returns the root model using the platform config directory.
func New() Model {
	return NewWithConfig(defaultConfig())
}

// NewWithConfig returns the root model with explicit paths.
func NewWithConfig(cfg Config) Model {
	h := help.New()
	h.Width = defaultWidth
	manager := db.NewManager()
	store := workspace.NewStore(cfg.ConnectionsPath)
	clipboard := cfg.Clipboard
	if clipboard == nil {
		clipboard = export.SystemClipboard{}
	}
	return Model{
		theme:     styles.Default(),
		help:      h,
		keys:      defaultKeyMap(),
		manager:   manager,
		workspace: workspace.New(store, manager),
		query:     query.New(),
		results:   result.New(),
		clipboard: clipboard,
		focus:     FocusWorkspace,
	}
}

func defaultConfig() Config {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}
	}
	return Config{ConnectionsPath: filepath.Join(dir, "sql-tui", "connections.json")}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.workspace.Init(), m.query.Init())
}

// Close releases every open database handle. Call it after the program exits.
func (m Model) Close() error {
	return m.manager.CloseAll()
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.resize()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case workspace.ActivatedMsg:
		connection := msg.Connection
		m.active = &connection
		m.focus = FocusQuery
		m.workspace.SetActive(connection.ID)
		m.syncFocus()
		return m, m.loadSchema(connection)
	case workspace.DeactivatedMsg:
		m.active = nil
		m.schema = nil
		m.schemaErr = nil
		return m, nil
	case schemaLoadedMsg:
		if m.active == nil || m.active.ID != msg.ConnectionID {
			return m, nil
		}
		m.schema = msg.Tables
		m.schemaErr = msg.Err
		return m, nil
	case query.ExecuteRequestedMsg:
		m.clearNotice()
		return m.startQuery(msg.SQL)
	case queryFinishedMsg:
		m.query.ReportSuccess(msg.Statements, msg.Result)
		m.results.SetResult(msg.Result)
		m.lastQueryErr = nil
		m.focus = FocusResult
		m.syncFocus()
		return m, nil
	case queryFailedMsg:
		m.query.ReportError(msg.Err)
		m.lastQueryErr = msg.Err
		return m, nil
	case exportDoneMsg:
		if msg.Err != nil {
			m.openModal("Export failed", msg.Err.Error())
			return m, nil
		}
		if msg.Clipboard {
			cmd := m.setNotice("Copied to clipboard")
			return m, cmd
		}
		cmd := m.setNotice("Export complete: " + msg.Path)
		return m, cmd
	case statusTimeoutMsg:
		if msg.Seq == m.noticeSeq {
			m.notice = ""
		}
		return m, nil
	case sessionEventMsg:
		return m.routeSessionEvent(msg)
	default:
		workspaceModel, workspaceCmd := m.workspace.Update(msg)
		queryModel, queryCmd := m.query.Update(msg)
		m.workspace = workspaceModel
		m.query = queryModel
		return m, tea.Batch(workspaceCmd, queryCmd)
	}
}

type queryFinishedMsg struct {
	Result     db.QueryResult
	Statements int
}

type queryFailedMsg struct {
	Err error
}

type exportDoneMsg struct {
	Path      string
	Clipboard bool
	Err       error
}

type statusTimeoutMsg struct {
	Seq int
}

// setNotice shows a transient footer message and schedules its expiry.
func (m *Model) setNotice(text string) tea.Cmd {
	m.notice = text
	m.noticeSeq++
	seq := m.noticeSeq
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return statusTimeoutMsg{Seq: seq} })
}

func (m *Model) clearNotice() {
	m.notice = ""
	m.noticeSeq++
}

func (m Model) submitExport() (tea.Model, tea.Cmd) {
	if !m.export.validate() {
		return m, nil
	}
	dialog := m.export
	m.export = nil

	payload, ok := m.results.Result()
	if !ok {
		cmd := m.setNotice("no results to export")
		return m, cmd
	}
	clipboard := m.clipboard
	format := dialog.format
	destination := dialog.destination
	path := dialog.path.Value()

	return m, func() tea.Msg {
		if destination == exportToClipboard {
			text, err := export.Serialize(format, payload)
			if err != nil {
				return exportDoneMsg{Err: err}
			}
			if err := clipboard.WriteAll(text); err != nil {
				return exportDoneMsg{Err: err}
			}
			return exportDoneMsg{Clipboard: true}
		}
		absolute, err := export.WriteFile(path, format, payload)
		return exportDoneMsg{Path: absolute, Err: err}
	}
}

func (m Model) startQuery(sql string) (tea.Model, tea.Cmd) {
	if m.active == nil {
		m.query.SetStatus("no connection selected")
		return m, nil
	}
	handle, ok := m.manager.Get(m.active.ID)
	if !ok {
		m.query.ReportError(errors.New("connection is not open"))
		return m, nil
	}
	m.query.MarkRunning()
	return m, m.executeQuery(handle, sql)
}

func (m Model) executeQuery(handle *sql.DB, sql string) tea.Cmd {
	return func() tea.Msg {
		statements := query.SplitStatements(sql)
		if len(statements) == 0 {
			return queryFailedMsg{Err: errors.New("no executable SQL")}
		}

		started := time.Now()
		var last db.QueryResult
		for i, statement := range statements {
			result := db.Execute(context.Background(), handle, statement, db.DefaultMaxRows)
			if result.Err != nil {
				return queryFailedMsg{Err: fmt.Errorf("statement %d: %w", i+1, result.Err)}
			}
			last = result
		}
		last.Elapsed = time.Since(started)
		return queryFinishedMsg{Result: last, Statements: len(statements)}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if m.export != nil {
		if key.Matches(msg, m.keys.Close) {
			m.export = nil
			return m, nil
		}
		if m.export.update(msg) {
			return m.submitExport()
		}
		return m, nil
	}
	if m.workspace.CapturesInput() {
		var cmd tea.Cmd
		m.workspace, cmd = m.workspace.Update(msg)
		return m, cmd
	}
	if m.modal != nil {
		if key.Matches(msg, m.keys.Close) {
			m.modal = nil
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Cycle):
		m.focus = m.focus.Next()
		m.syncFocus()
		return m, nil
	case key.Matches(msg, m.keys.CyclePrev):
		m.focus = m.focus.Prev()
		m.syncFocus()
		return m, nil
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.resize()
		return m, nil
	case key.Matches(msg, m.keys.Export):
		if m.focus != FocusResult {
			return m, nil
		}
		if _, ok := m.results.Result(); !ok {
			cmd := m.setNotice("no results to export")
			return m, cmd
		}
		m.export = newExportDialog()
		return m, nil
	}
	if m.focus == FocusWorkspace {
		var cmd tea.Cmd
		m.workspace, cmd = m.workspace.Update(msg)
		return m, cmd
	}
	if m.focus == FocusQuery {
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		return m, cmd
	}
	if m.focus == FocusResult {
		var cmd tea.Cmd
		m.results, cmd = m.results.Update(msg)
		return m, cmd
	}
	return m, nil
}

// syncFocus mirrors the pane focus onto child widgets that track it.
func (m *Model) syncFocus() {
	if m.focus == FocusResult {
		m.results.Focus()
		return
	}
	m.results.Blur()
}

// routeSessionEvent dispatches an async query event to its owning session.
// Sessions are introduced in §11; until then events are dropped.
func (m Model) routeSessionEvent(sessionEventMsg) (tea.Model, tea.Cmd) {
	return m, nil
}

type schemaLoadedMsg struct {
	ConnectionID string
	Tables       []db.TableMeta
	Err          error
}

func (m Model) loadSchema(connection db.Connection) tea.Cmd {
	manager := m.manager
	return func() tea.Msg {
		handle, ok := manager.Get(connection.ID)
		if !ok {
			return schemaLoadedMsg{ConnectionID: connection.ID, Err: errors.New("connection is not open")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
		defer cancel()
		tables, err := db.LoadSchema(ctx, handle, connection.Driver)
		return schemaLoadedMsg{ConnectionID: connection.ID, Tables: tables, Err: err}
	}
}

// openModal shows a centred modal and dims the panes behind it.
func (m *Model) openModal(title, body string) {
	m.modal = &modalBox{Title: title, Body: body}
}

func (m *Model) closeModal() {
	m.modal = nil
}

// View implements tea.Model.
func (m Model) View() string {
	lay := m.layout()

	workspaceView := m.pane("Connections", m.workspace.View(), lay.workspaceW, lay.bodyH, m.focus == FocusWorkspace)
	queryView := m.pane("Query", m.query.View(), lay.rightW, lay.queryH, m.focus == FocusQuery)
	results := m.pane("Results", m.resultsView(lay.rightW-4), lay.rightW, lay.resultH, m.focus == FocusResult)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		workspaceView,
		lipgloss.JoinVertical(lipgloss.Left, queryView, results),
	)
	status := m.theme.Status.Width(lay.width).Render(m.statusLine(lay))
	helpView := m.theme.Help.Width(lay.width).Render(m.help.View(m.keys))

	view := lipgloss.JoinVertical(lipgloss.Left, body, status, helpView)
	if title, body, ok := m.workspace.Modal(); ok {
		view = m.renderModal(view, lay, modalBox{Title: title, Body: body})
	} else if m.export != nil {
		view = m.renderModal(view, lay, modalBox{Title: "Export results", Body: m.export.view()})
	} else if m.modal != nil {
		view = m.renderModal(view, lay, *m.modal)
	}
	return view
}

func (m Model) statusLine(lay layout) string {
	parts := []string{"sql-tui", "focus: " + m.focus.String()}
	if m.active != nil {
		parts = append(parts, "conn: "+m.active.Name)
	}
	if len(m.schema) > 0 {
		parts = append(parts, fmt.Sprintf("schema: %d objects", len(m.schema)))
	}
	parts = append(parts, fmt.Sprintf("%d×%d", lay.width, lay.height))
	line := strings.Join(parts, " · ")
	if m.notice != "" {
		line = m.notice + " · " + line
	}
	return truncate(line, lay.width)
}

// resize propagates the current pane geometry to child models. Pane content
// includes the title row, so children get height-3 (border and title removed).
func (m *Model) resize() {
	lay := m.layout()
	m.workspace.SetSize(max(lay.workspaceW-4, 1), max(lay.bodyH-3, 1))
	m.query.SetSize(max(lay.rightW-4, 1), max(lay.queryH-3, 1))
	m.results.SetSize(max(lay.rightW-4, 1), max(lay.resultH-3, 1))
	if m.active != nil {
		m.workspace.SetActive(m.active.ID)
	}
}

func (m Model) resultsView(width int) string {
	if m.query.Running() {
		return "running…"
	}
	if m.lastQueryErr != nil {
		return "error: " + truncate(m.lastQueryErr.Error(), width)
	}
	return m.results.View()
}

func truncate(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.Truncate(text, width, "…")
}

type layout struct {
	width      int
	height     int
	workspaceW int
	rightW     int
	bodyH      int
	queryH     int
	resultH    int
}

func (m Model) layout() layout {
	w := m.width
	if w <= 0 {
		w = defaultWidth
	}
	h := m.height
	if h <= 0 {
		h = defaultHeight
	}

	helpH := lipgloss.Height(m.help.View(m.keys))
	if helpH < 1 {
		helpH = 1
	}
	bodyH := h - 1 - helpH
	if bodyH < minBodyHeight {
		bodyH = minBodyHeight
	}

	ws := workspaceWidth
	if ws > w*2/5 {
		ws = w * 2 / 5
	}
	if ws < minWorkspaceWidth {
		ws = minWorkspaceWidth
	}
	if ws > w-1 {
		ws = w / 2
	}
	rightW := w - ws
	if rightW < 1 {
		rightW = 1
	}

	queryH := bodyH / 2
	resultH := bodyH - queryH
	if resultH < 1 {
		resultH = 1
		queryH = bodyH - 1
	}

	return layout{
		width:      w,
		height:     h,
		workspaceW: ws,
		rightW:     rightW,
		bodyH:      bodyH,
		queryH:     queryH,
		resultH:    resultH,
	}
}

func (m Model) pane(title, body string, w, h int, focused bool) string {
	style := m.theme.Pane
	if focused {
		style = m.theme.PaneFocused
	}
	content := m.theme.PaneTitle.Render(title) + "\n" + body
	return style.Width(max(w-2, 1)).Height(max(h-2, 1)).Render(content)
}

func (m Model) renderModal(background string, lay layout, box modalBox) string {
	boxWidth := min(56, max(lay.width-8, 8))
	content := m.theme.ModalTitle.Render(box.Title) + "\n\n" + box.Body
	rendered := m.theme.Modal.Width(boxWidth).Render(content)
	return overlay(m.dimLines(background), rendered, lay.width)
}

func (m Model) dimLines(background string) string {
	lines := strings.Split(background, "\n")
	for i := range lines {
		lines[i] = m.theme.Dim.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}
