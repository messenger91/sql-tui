package app

import (
	"context"
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
	"github.com/messenger91/sql-tui/internal/favorites"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/schema"
	"github.com/messenger91/sql-tui/internal/session"
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
	FavoritesPath   string
	Clipboard       export.Clipboard
}

// Model is the root application model.
type Model struct {
	theme     styles.Theme
	help      help.Model
	keys      keyMap
	manager   *db.Manager
	workspace workspace.Model
	favorites favorites.Model
	schema    *schema.Cache
	clipboard export.Clipboard
	sessions  *session.Manager

	focus         Focus
	width, height int
	modal         *modalBox
	export        *exportDialog
	browser       *schemaBrowser
	notice        string
	noticeSeq     int

	active       *db.Connection
	schemaLoad   bool
	confirmClose session.ID

	sender          *senderHolder
	runners         *runnerGroup
	cancelRequested bool
	quitting        bool
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
	favoriteStore := favorites.NewStore(cfg.FavoritesPath)
	clipboard := cfg.Clipboard
	if clipboard == nil {
		clipboard = export.SystemClipboard{}
	}
	sessions := session.NewManager()
	sessions.New("")
	return Model{
		theme:     styles.Default(),
		help:      h,
		keys:      defaultKeyMap(),
		manager:   manager,
		workspace: workspace.New(store, manager),
		favorites: favorites.New(favoriteStore),
		schema:    schema.NewCache(),
		clipboard: clipboard,
		sessions:  sessions,
		sender:    newSenderHolder(),
		runners:   newRunnerGroup(),
		focus:     FocusWorkspace,
	}
}

func defaultConfig() Config {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}
	}
	base := filepath.Join(dir, "sql-tui")
	return Config{
		ConnectionsPath: filepath.Join(base, "connections.json"),
		FavoritesPath:   filepath.Join(base, "favorites.json"),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.workspace.Init(), m.favorites.Init()}
	if s := m.activeSession(); s != nil {
		cmds = append(cmds, s.Editor.Init())
	}
	return tea.Batch(cmds...)
}

// activeSession returns the focused session. The app always keeps at least one
// session alive, so callers may assume non-nil outside of construction.
func (m Model) activeSession() *session.Session {
	if s, ok := m.sessions.Active(); ok {
		return s
	}
	return nil
}

// Close releases every open database handle. Call it after the program exits.
func (m Model) Close() error {
	m.sender.clear()
	for _, s := range m.sessions.Sessions() {
		s.Cancel()
	}
	m.runners.wait()
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
		m.workspace.ActivationSucceeded(connection.ID)
		m.focusSessionForConnection(connection.ID)
		m.syncFocus()
		cmd := m.refreshSchema(connection)
		return m, cmd
	case workspace.DeactivatedMsg:
		m.active = nil
		m.schemaLoad = false
		return m, nil
	case schemaLoadedMsg:
		m.schema.Store(msg.ConnectionID, msg.Tables, msg.Err)
		entry, ok := m.schema.Get(msg.ConnectionID)
		for _, s := range m.sessions.Sessions() {
			if s.ConnectionID == msg.ConnectionID {
				if ok {
					s.Editor.SetSchema(entry.Index)
				} else {
					s.Editor.SetSchema(nil)
				}
			}
		}
		if m.active != nil && m.active.ID == msg.ConnectionID {
			m.schemaLoad = false
			if m.browser != nil {
				m.browser.SetTables(msg.Tables)
			}
		}
		return m, nil
	case query.ExecuteRequestedMsg:
		m.clearNotice()
		return m.startQuery(msg.SQL)
	case favorites.LoadMsg:
		if s := m.activeSession(); s != nil {
			s.SetSQL(msg.SQL)
		}
		m.focus = FocusQuery
		m.syncFocus()
		cmd := m.setNotice("loaded favorite: " + msg.Name)
		return m, cmd
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
		return m.handleRunnerEvent(msg)
	case shutdownDoneMsg:
		return m, tea.Quit
	default:
		workspaceModel, workspaceCmd := m.workspace.Update(msg)
		favoritesModel, favoritesCmd := m.favorites.Update(msg)
		m.workspace = workspaceModel
		m.favorites = favoritesModel

		var sessionCmd tea.Cmd
		if s := m.activeSession(); s != nil {
			editor, cmd := s.Editor.Update(msg)
			s.Editor = editor
			sessionCmd = cmd
		}
		return m, tea.Batch(workspaceCmd, favoritesCmd, sessionCmd)
	}
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

	s := m.activeSession()
	if s == nil {
		cmd := m.setNotice("no results to export")
		return m, cmd
	}
	payload, ok := s.Results.Result()
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
	s := m.activeSession()
	if s == nil {
		return m, nil
	}
	if s.Running() {
		return m, nil
	}
	connectionID := s.ConnectionID
	if connectionID == "" && m.active != nil {
		connectionID = m.active.ID
		s.ConnectionID = connectionID
	}
	if connectionID == "" {
		s.Editor.SetStatus("no connection selected")
		return m, nil
	}
	handle, ok := m.manager.Get(connectionID)
	if !ok {
		s.Editor.ReportError(errors.New("connection is not open"))
		s.MarkFailed()
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.Title = session.DeriveTitle(sql)
	s.MarkRunning(cancel)
	s.Editor.MarkRunning()
	m.cancelRequested = false
	m.quitting = false

	events := make(chan session.Event, 8)
	sessionID := s.ID
	m.runners.begin(2)

	go func() {
		defer m.runners.done()
		defer close(events)
		session.Run(ctx, handle, sql, db.DefaultMaxRows, func(event session.Event) bool {
			switch event.Kind {
			case session.EventDone, session.EventError, session.EventCancelled:
				events <- event
				return true
			}
			select {
			case events <- event:
				return true
			case <-ctx.Done():
				return false
			}
		})
	}()

	go func() {
		defer m.runners.done()
		for event := range events {
			m.sender.publish(sessionEventMsg{SessionID: sessionID, Event: event})
		}
	}()

	return m, nil
}

func (m Model) handleRunnerEvent(msg sessionEventMsg) (tea.Model, tea.Cmd) {
	s, ok := m.sessions.Get(msg.SessionID)
	if !ok {
		return m, nil
	}
	event, ok := msg.Event.(session.Event)
	if !ok {
		return m, nil
	}

	switch event.Kind {
	case session.EventStarted:
		s.Results.SetStreaming(true)
		s.Results.SetResult(db.QueryResult{Columns: event.Columns})
	case session.EventBatch:
		s.Results.AppendRows(event.Rows)
	case session.EventDone:
		s.Results.SetStreaming(false)
		s.Editor.ReportSuccess(event.Statements, db.QueryResult{
			RowCount:  event.RowCount,
			Elapsed:   event.Elapsed,
			Truncated: event.Truncated,
		})
		s.MarkDone()
		if active := m.activeSession(); active != nil && active.ID == s.ID {
			m.focus = FocusResult
			m.syncFocus()
		}
	case session.EventError:
		s.Results.SetStreaming(false)
		s.Editor.ReportError(event.Err)
		s.MarkFailed()
	case session.EventCancelled:
		s.Results.SetStreaming(false)
		s.Editor.SetStatus("cancelled")
		s.MarkCancelled()
	}
	return m, nil
}

type shutdownDoneMsg struct{}

// shutdownCmd stops every runner before the program exits, so forwarders never
// block in tea.Program.Send after the event loop is gone.
func (m *Model) shutdownCmd() tea.Cmd {
	m.sender.clear()
	for _, s := range m.sessions.Sessions() {
		s.Cancel()
		s.MarkCancelled()
		s.Results.SetStreaming(false)
	}
	return func() tea.Msg {
		m.runners.wait()
		return shutdownDoneMsg{}
	}
}

func (m *Model) cancelSession(s *session.Session) {
	s.Cancel()
	s.MarkCancelled()
	s.Editor.SetStatus("cancelled")
	s.Results.SetStreaming(false)
	m.cancelRequested = true
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		if m.quitting {
			return m, nil
		}
		if s := m.activeSession(); s != nil && s.Running() && !m.cancelRequested {
			m.cancelSession(s)
			cmd := m.setNotice("cancelling…")
			return m, cmd
		}
		if m.runners.count() > 0 {
			m.quitting = true
			return m, m.shutdownCmd()
		}
		return m, tea.Quit
	}
	if m.confirmClose != "" {
		switch msg.String() {
		case "y", "Y":
			id := m.confirmClose
			m.confirmClose = ""
			m.closeSession(id)
		case "n", "N", "esc":
			m.confirmClose = ""
		}
		return m, nil
	}
	// The autocomplete popup owns tab (accept), runes (continue typing), and
	// must not let global bindings swallow them.
	if m.focus == FocusQuery {
		if s := m.activeSession(); s != nil && s.Editor.PopupVisible() {
			if key.Matches(msg, m.keys.Cycle) || msg.Type == tea.KeyRunes {
				editor, cmd := s.Editor.Update(msg)
				s.Editor = editor
				return m, cmd
			}
		}
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
	if m.favorites.CapturesInput() {
		var cmd tea.Cmd
		m.favorites, cmd = m.favorites.Update(msg)
		return m, cmd
	}
	if m.browser != nil {
		return m.handleBrowserKey(msg)
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
	if msg.String() == "esc" {
		if s := m.activeSession(); s != nil && s.Running() {
			m.cancelSession(s)
			cmd := m.setNotice("cancelling…")
			return m, cmd
		}
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
	case key.Matches(msg, m.keys.NewTab):
		m.newTab()
		return m, nil
	case key.Matches(msg, m.keys.CloseTab):
		if s := m.activeSession(); s != nil {
			if s.Running() {
				m.confirmClose = s.ID
			} else {
				m.closeSession(s.ID)
			}
		}
		return m, nil
	case key.Matches(msg, m.keys.NextTab):
		m.sessions.Next()
		m.afterTabSwitch()
		return m, nil
	case key.Matches(msg, m.keys.PrevTab):
		m.sessions.Prev()
		m.afterTabSwitch()
		return m, nil
	case key.Matches(msg, m.keys.JumpTab):
		m.jumpToTab(msg.String())
		return m, nil
	case key.Matches(msg, m.keys.ViewFormat) && m.focus == FocusResult:
		if s := m.activeSession(); s != nil {
			s.Results.CycleFormat()
		}
		return m, nil
	case key.Matches(msg, m.keys.SaveFav) && m.focus == FocusQuery:
		s := m.activeSession()
		if s == nil {
			return m, nil
		}
		if strings.TrimSpace(s.Editor.SQL()) == "" {
			cmd := m.setNotice("nothing to save")
			return m, cmd
		}
		m.favorites.OpenSave(s.Editor.SQL())
		return m, nil
	case key.Matches(msg, m.keys.BrowseFav) && m.focus == FocusQuery:
		m.favorites.OpenBrowse()
		return m, nil
	case key.Matches(msg, m.keys.BrowseSchema):
		if m.active == nil {
			cmd := m.setNotice("select a connection first")
			return m, cmd
		}
		if m.schemaLoad {
			cmd := m.setNotice("schema still loading")
			return m, cmd
		}
		entry, ok := m.schema.Get(m.active.ID)
		if !ok {
			cmd := m.setNotice("schema not loaded")
			return m, cmd
		}
		browser := newSchemaBrowser()
		browser.SetSize(m.layout().width, m.layout().height)
		browser.Open(entry.Tables)
		m.browser = browser
		return m, nil
	case key.Matches(msg, m.keys.Export) && m.focus == FocusResult:
		s := m.activeSession()
		if s == nil || s.Running() {
			cmd := m.setNotice("wait for the query to finish")
			return m, cmd
		}
		if _, ok := s.Results.Result(); !ok {
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
		if s := m.activeSession(); s != nil {
			editor, cmd := s.Editor.Update(msg)
			s.Editor = editor
			return m, cmd
		}
	}
	if m.focus == FocusResult {
		if s := m.activeSession(); s != nil {
			results, cmd := s.Results.Update(msg)
			s.Results = results
			return m, cmd
		}
	}
	return m, nil
}

// syncFocus mirrors the pane focus onto session result widgets.
func (m *Model) syncFocus() {
	active, ok := m.sessions.Active()
	for _, s := range m.sessions.Sessions() {
		if ok && s.ID == active.ID && m.focus == FocusResult {
			s.Results.Focus()
			continue
		}
		s.Results.Blur()
	}
}

func (m *Model) afterTabSwitch() {
	m.applySchemaToActive()
	m.syncFocus()
}

func (m *Model) jumpToTab(key string) {
	if len(key) == 0 {
		return
	}
	digit := key[len(key)-1]
	if digit < '1' || digit > '9' {
		return
	}
	index := int(digit - '1')
	order := m.sessions.Sessions()
	if index >= len(order) {
		return
	}
	m.sessions.SetActive(order[index].ID)
	m.afterTabSwitch()
}

func (m *Model) focusSessionForConnection(connectionID string) {
	if s := m.activeSession(); s != nil && s.ConnectionID == "" && !s.Running() {
		s.ConnectionID = connectionID
		m.applySchema(s)
		return
	}
	for _, s := range m.sessions.Sessions() {
		if s.ConnectionID == connectionID {
			m.sessions.SetActive(s.ID)
			m.applySchema(s)
			return
		}
	}
	s := m.sessions.New(connectionID)
	m.applySchema(s)
}

func (m *Model) newTab() {
	connectionID := ""
	if s := m.activeSession(); s != nil {
		connectionID = s.ConnectionID
	}
	if connectionID == "" && m.active != nil {
		connectionID = m.active.ID
	}
	s := m.sessions.New(connectionID)
	m.applySchema(s)
	m.focus = FocusQuery
	m.syncFocus()
}

func (m *Model) closeSession(id session.ID) {
	m.sessions.Close(id)
	if m.sessions.Len() == 0 {
		connectionID := ""
		if m.active != nil {
			connectionID = m.active.ID
		}
		s := m.sessions.New(connectionID)
		m.applySchema(s)
	}
	m.syncFocus()
}

func (m *Model) applySchema(s *session.Session) {
	if s == nil || s.ConnectionID == "" {
		if s != nil {
			s.Editor.SetSchema(nil)
		}
		return
	}
	entry, ok := m.schema.Get(s.ConnectionID)
	if !ok {
		s.Editor.SetSchema(nil)
		return
	}
	s.Editor.SetSchema(entry.Index)
}

func (m *Model) applySchemaToActive() {
	m.applySchema(m.activeSession())
}

func (m Model) handleBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.browser.Update(msg)
	switch {
	case action.insert != "":
		if s := m.activeSession(); s != nil {
			s.Editor.InsertText(action.insert)
		}
		m.focus = FocusQuery
		m.syncFocus()
		m.browser = nil
	case action.close:
		m.browser = nil
	case action.refresh:
		if m.active != nil {
			cmd := m.refreshSchema(*m.active)
			return m, cmd
		}
	}
	return m, nil
}

type schemaLoadedMsg struct {
	ConnectionID string
	Tables       []db.TableMeta
	Err          error
}

// refreshSchema starts an async schema reload and marks the status as loading.
// It is used on connection activation and by the schema browser (§10.2).
func (m *Model) refreshSchema(connection db.Connection) tea.Cmd {
	m.schemaLoad = true
	return m.loadSchema(connection)
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

	var queryContent, resultsContent string
	if s := m.activeSession(); s != nil {
		queryContent = m.tabBar(max(lay.rightW-4, 1)) + "\n" + s.Editor.View()
		resultsContent = s.Results.View()
	} else {
		queryContent = "no session"
		resultsContent = "no results yet"
	}

	workspaceView := m.pane("Connections", m.workspace.View(), lay.workspaceW, lay.bodyH, m.focus == FocusWorkspace)
	queryView := m.pane("Query", queryContent, lay.rightW, lay.queryH, m.focus == FocusQuery)
	results := m.pane("Results", resultsContent, lay.rightW, lay.resultH, m.focus == FocusResult)

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
	} else if title, body, ok := m.favorites.Modal(); ok {
		view = m.renderModal(view, lay, modalBox{Title: title, Body: body})
	} else if m.browser != nil {
		view = m.renderModal(view, lay, modalBox{Title: m.browser.Title(), Body: m.browser.View()})
	} else if m.confirmClose != "" {
		view = m.renderModal(view, lay, modalBox{
			Title: "Close tab",
			Body:  "The query in this tab is running.\n\ny close · n cancel",
		})
	} else if m.modal != nil {
		view = m.renderModal(view, lay, *m.modal)
	}
	return view
}

func (m Model) tabBar(width int) string {
	sessions := m.sessions.Sessions()
	active, _ := m.sessions.Active()
	parts := make([]string, 0, len(sessions)+1)
	for i, s := range sessions {
		label := fmt.Sprintf("%d: ", i+1)
		if s.Running() {
			label += "● "
		}
		label += s.Title
		style := m.theme.Status
		if active != nil && s.ID == active.ID {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
		}
		parts = append(parts, style.Render(label))
	}
	parts = append(parts, m.theme.PaneTitle.Render("+"))
	return truncate(strings.Join(parts, m.theme.Status.Render(" │ ")), width)
}

func (m Model) statusLine(lay layout) string {
	parts := []string{"sql-tui", "focus: " + m.focus.String()}
	if m.active != nil {
		parts = append(parts, "conn: "+m.active.Name)
		switch {
		case m.schemaLoad:
			parts = append(parts, "schema: loading…")
		default:
			if entry, ok := m.schema.Get(m.active.ID); ok {
				switch {
				case len(entry.Tables) > 0 && entry.Err != nil:
					parts = append(parts, "schema: "+objectsLabel(len(entry.Tables))+" (partial)")
				case len(entry.Tables) > 0:
					parts = append(parts, "schema: "+objectsLabel(len(entry.Tables)))
				case entry.Err != nil:
					parts = append(parts, "schema: unavailable")
				}
			}
		}
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
	for _, s := range m.sessions.Sessions() {
		s.Editor.SetSize(max(lay.rightW-4, 1), max(lay.queryH-4, 1)) // tab bar row
		s.Results.SetSize(max(lay.rightW-4, 1), max(lay.resultH-3, 1))
	}
	m.favorites.SetSize(lay.width, lay.height)
	if m.browser != nil {
		m.browser.SetSize(lay.width, lay.height)
	}
	if m.active != nil {
		m.workspace.SetActive(m.active.ID)
	}
}

func truncate(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.Truncate(text, width, "…")
}

func objectsLabel(count int) string {
	if count == 1 {
		return "1 object"
	}
	return fmt.Sprintf("%d objects", count)
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
