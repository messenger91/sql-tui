package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/messenger91/sql-tui/internal/history"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/schema"
	"github.com/messenger91/sql-tui/internal/session"
	"github.com/messenger91/sql-tui/internal/settings"
	"github.com/messenger91/sql-tui/internal/styles"
	"github.com/messenger91/sql-tui/internal/workspace"
)

const (
	defaultWidth    = 80
	defaultHeight   = 24
	workspaceWidth  = 30
	minBodyHeight   = 3
	schemaTimeout   = 10 * time.Second
	fetchTableLimit = 100

	// Below this width the workspace pane is hidden entirely.
	collapseWidth = 25
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
	HistoryPath     string
	SettingsPath    string
	Clipboard       export.Clipboard
}

// Version is the application version shown in the status bar and --version.
const Version = "0.1.0"

// Model is the root application model.
type Model struct {
	theme        styles.Theme
	help         help.Model
	keys         keyMap
	manager      *db.Manager
	workspace    workspace.Model
	favorites    favorites.Model
	schema       *schema.Cache
	clipboard    export.Clipboard
	sessions     *session.Manager
	history      []string
	historyStore *history.Store
	settings     settings.Settings
	config       Config

	focus         Focus
	width, height int
	wsHidden      bool
	modal         *modalBox
	export        *exportDialog
	browser       *schemaBrowser
	cellEdit      *cellEditor
	rowMenu       *rowMenu
	preview       *previewState
	helpOpen      bool
	menu          *commandMenuState
	needsRefresh  bool
	notice        string
	noticeSeq     int

	active       *db.Connection
	schemaLoad   bool
	confirmClose session.ID
	doc          *docViewer

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
		theme:        styles.Default(),
		help:         h,
		keys:         defaultKeyMap(),
		manager:      manager,
		workspace:    workspace.New(store, manager),
		favorites:    favorites.New(favoriteStore),
		schema:       schema.NewCache(),
		clipboard:    clipboard,
		sessions:     sessions,
		sender:       newSenderHolder(),
		runners:      newRunnerGroup(),
		historyStore: history.NewStore(cfg.HistoryPath),
		settings:     settings.Default(),
		config:       cfg,
		focus:        FocusWorkspace,
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
		HistoryPath:     filepath.Join(base, "history.json"),
		SettingsPath:    filepath.Join(base, settings.FileName),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.workspace.Init(), m.favorites.Init(), m.loadHistory(), m.loadSettings()}
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
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case workspace.ActivatedMsg:
		connection := msg.Connection
		m.active = &connection
		m.focus = FocusQuery
		m.workspace.ActivationSucceeded(connection.ID)
		m.focusSessionForConnection(connection)
		m.syncFocus()
		cmd := m.refreshSchema(connection)
		return m, cmd
	case workspace.DeactivatedMsg:
		m.active = nil
		m.schemaLoad = false
		return m, nil
	case schemaLoadedMsg:
		if os.Getenv("SQL_TUI_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "schemaLoaded for", msg.ConnectionID, "err:", msg.Err)
		}
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
		m.workspace.SetTables(schemaTableNames(msg.Tables))
		if m.active != nil && m.active.ID == msg.ConnectionID {
			m.schemaLoad = false
			if m.browser != nil {
				m.browser.SetTables(msg.Tables)
			}
		}
		return m, nil
	case settingsLoadedMsg:
		m.settings = msg.Settings.Sanitized()
		return m, nil
	case historyLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.history = msg.Queries
		for _, s := range m.sessions.Sessions() {
			s.Editor.SetHistory(m.history)
		}
		return m, nil
	case query.ExecuteRequestedMsg:
		return m.executeSQL(msg.SQL)
	case workspace.FetchTableMsg:
		return m.executeSQL(m.fetchTableSQL(msg.Table))
	case workspace.TableInfoMsg:
		return m.startTableInfo(msg.Table)
	case workspace.DBMLMsg:
		return m.openDBML()
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
	case applyDoneMsg:
		return m.handleApplyDone(msg)
	case tableInfoDoneMsg:
		return m.handleTableInfoDone(msg)
	case docCopiedMsg:
		if msg.Err != nil {
			m.openModal("Copy failed", msg.Err.Error())
			return m, nil
		}
		cmd := m.setNotice("Copied to clipboard")
		return m, cmd
	case explainDoneMsg:
		if msg.Err != nil {
			m.openModal("Explain failed", msg.Err.Error())
			return m, nil
		}
		body := msg.Plan
		if strings.TrimSpace(body) == "" {
			body = "the database returned an empty plan"
		}
		m.openModal("Query plan · "+msg.Title, body)
		return m, nil
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

// schemaTableNames lists tables/views sorted by name.
func schemaTableNames(tables []db.TableMeta) []string {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}
	sort.Strings(names)
	return names
}

// fetchTableSQL builds the quick-fetch query for a table dropdown selection.
func (m Model) fetchTableSQL(table string) string {
	driver := db.Driver("")
	if s := m.activeSession(); s != nil {
		driver = s.Driver
	} else if m.active != nil {
		driver = m.active.Driver
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d",
		driver.Dialect().QuoteIdent(table), fetchTableLimit)
}

// executeSQL is the shared path for editor runs and quick fetches.
func (m Model) executeSQL(sql string) (tea.Model, tea.Cmd) {
	m.clearNotice()
	updated, cmd := m.startQuery(sql)
	model := updated.(Model)
	if s := model.activeSession(); s != nil && s.Running() {
		return model, tea.Batch(cmd, model.recordHistory(sql))
	}
	return model, cmd
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
	s.LastSQL = sql
	s.Editability = session.Editability{}
	s.ResetPending()
	m.syncPending(s)
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
		session.Run(ctx, handle, sql, m.effectiveMaxRows(), func(event session.Event) bool {
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
		if os.Getenv("SQL_TUI_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "event started", msg.SessionID)
		}
		s.Results.SetStreaming(true)
		s.Results.SetResult(db.QueryResult{Columns: event.Columns})
	case session.EventBatch:
		s.Results.AppendRows(event.Rows)
	case session.EventDone:
		if os.Getenv("SQL_TUI_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "event done", msg.SessionID, event.Fetched)
		}
		s.Results.SetStreaming(false)
		s.Results.UpdateStats(event.RowCount, event.Fetched, event.Elapsed, event.Truncated)
		s.Editor.ReportSuccess(event.Statements, db.QueryResult{
			RowCount:  event.RowCount,
			Fetched:   event.Fetched,
			Elapsed:   event.Elapsed,
			Truncated: event.Truncated,
		})
		s.MarkDone()
		s.Editability = m.computeEditability(s)
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
	if m.helpOpen {
		return m.handleHelpKey(msg)
	}
	if m.doc != nil {
		return m.handleDocKey(msg)
	}
	if m.menu != nil {
		return m.handleCommandMenuKey(msg)
	}
	if m.cellEdit != nil {
		return m.handleCellEditorKey(msg)
	}
	if m.rowMenu != nil {
		return m.handleRowMenuKey(msg)
	}
	if m.preview != nil {
		return m.handlePreviewKey(msg)
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
	if m.paneHotkeysActive() {
		switch msg.String() {
		case "E":
			if m.wsHidden {
				return m, nil
			}
			m.focus = FocusWorkspace
			m.syncFocus()
			return m, nil
		case "Q":
			m.focus = FocusQuery
			m.syncFocus()
			return m, nil
		case "R":
			m.focus = FocusResult
			m.syncFocus()
			return m, nil
		}
	}
	if msg.String() == "esc" {
		if s := m.activeSession(); s != nil && s.Running() {
			m.cancelSession(s)
			cmd := m.setNotice("cancelling…")
			return m, cmd
		}
	}
	if m.focus == FocusResult && msg.String() == "s" {
		if s := m.activeSession(); s != nil && s.Results.Format() == result.FormatTable {
			if _, name, _, ok := s.Results.SelectedCell(); ok {
				m.sortResultsByColumn(s, name)
			}
			return m, nil
		}
	}
	if m.focus == FocusResult {
		s := m.activeSession()
		if s != nil && m.needsRefresh && msg.String() == "r" {
			s.Pending.RevertAll()
			m.syncPending(s)
			m.needsRefresh = false
			if s.LastSQL != "" {
				updated, cmd := m.startQuery(s.LastSQL)
				return updated, cmd
			}
			cmd := m.setNotice("refreshed")
			return m, cmd
		}
	}
	switch {
	case key.Matches(msg, m.keys.Cycle):
		m.focus = m.focus.Next()
		m.skipHiddenWorkspace(true)
		m.syncFocus()
		return m, nil
	case key.Matches(msg, m.keys.CyclePrev):
		m.focus = m.focus.Prev()
		m.skipHiddenWorkspace(false)
		m.syncFocus()
		return m, nil
	case key.Matches(msg, m.keys.Help) && (m.focus != FocusQuery || m.editorNormalMode()):
		m.helpOpen = true
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
		return m.openSaveFavorite()
	case key.Matches(msg, m.keys.BrowseFav) && m.focus == FocusQuery:
		m.favorites.OpenBrowse()
		return m, nil
	case key.Matches(msg, m.keys.Explain) && m.focus == FocusResult:
		return m.startExplain()
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
		return m.openExport()
	case key.Matches(msg, m.keys.Command) && m.focus != FocusQuery:
		return m.openCommandMenu()
	}
	if m.focus == FocusResult {
		s := m.activeSession()
		if s != nil && s.Results.Format() == result.FormatTable {
			if s.Editability.Editable {
				switch msg.String() {
				case "e", "enter":
					if m.beginCellEdit(s) {
						return m, nil
					}
				case "m":
					if row, _, _, ok := s.Results.SelectedCell(); ok {
						m.rowMenu = &rowMenu{row: row}
						return m, nil
					}
				case "p":
					return m.openPreview(s)
				case "ctrl+z":
					if row, _, _, ok := s.Results.SelectedCell(); ok {
						s.Pending.RevertRow(row)
						m.syncPending(s)
					}
					return m, nil
				case "ctrl+shift+z":
					s.Pending.RevertAll()
					m.syncPending(s)
					return m, nil
				}
			} else if msg.String() == "e" {
				if _, _, _, ok := s.Results.SelectedCell(); ok {
					cmd := m.setNotice("result is read-only")
					return m, cmd
				}
			}
		}
	}
	if m.focus == FocusWorkspace && !m.wsHidden {
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

// skipHiddenWorkspace keeps focus out of the workspace pane when it is
// collapsed away on very narrow terminals.
func (m *Model) skipHiddenWorkspace(forward bool) {
	if !m.wsHidden || m.focus != FocusWorkspace {
		return
	}
	if forward {
		m.focus = FocusQuery
		return
	}
	m.focus = FocusResult
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

func (m *Model) focusSessionForConnection(connection db.Connection) {
	if s := m.activeSession(); s != nil && s.ConnectionID == "" && !s.Running() {
		s.ConnectionID = connection.ID
		s.ReadOnly = connection.ReadOnly
		s.Driver = connection.Driver
		s.Editor.SetHistory(m.history)
		m.applySchema(s)
		return
	}
	for _, s := range m.sessions.Sessions() {
		if s.ConnectionID == connection.ID {
			s.ReadOnly = connection.ReadOnly
			s.Driver = connection.Driver
			m.sessions.SetActive(s.ID)
			m.applySchema(s)
			return
		}
	}
	s := m.sessions.New(connection.ID)
	s.ReadOnly = connection.ReadOnly
	s.Driver = connection.Driver
	s.Editor.SetHistory(m.history)
	m.applySchema(s)
}

func (m *Model) newTab() {
	connectionID := ""
	readOnly := true
	driver := db.Driver("")
	if s := m.activeSession(); s != nil {
		connectionID = s.ConnectionID
		readOnly = s.ReadOnly
		driver = s.Driver
	}
	if connectionID == "" && m.active != nil {
		connectionID = m.active.ID
		readOnly = m.active.ReadOnly
		driver = m.active.Driver
	}
	s := m.sessions.New(connectionID)
	s.ReadOnly = readOnly
	s.Driver = driver
	s.Editor.SetHistory(m.history)
	m.applySchema(s)
	m.focus = FocusQuery
	m.syncFocus()
}

func (m *Model) closeSession(id session.ID) {
	m.sessions.Close(id)
	if m.sessions.Len() == 0 {
		connectionID := ""
		readOnly := true
		driver := db.Driver("")
		if m.active != nil {
			connectionID = m.active.ID
			readOnly = m.active.ReadOnly
			driver = m.active.Driver
		}
		s := m.sessions.New(connectionID)
		s.ReadOnly = readOnly
		s.Driver = driver
		s.Editor.SetHistory(m.history)
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

func (m Model) openSaveFavorite() (tea.Model, tea.Cmd) {
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
}

func (m Model) openExport() (tea.Model, tea.Cmd) {
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

func (m Model) openSchemaBrowser() (tea.Model, tea.Cmd) {
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
}

// computeEditability evaluates whether a session's result can be edited (§12).
func (m Model) computeEditability(s *session.Session) session.Editability {
	if s == nil || s.LastSQL == "" {
		return session.Editability{}
	}
	entry, ok := m.schema.Get(s.ConnectionID)
	if !ok {
		return session.ComputeEditability(s.LastSQL, nil, nil, s.ReadOnly)
	}
	result, hasResult := s.Results.Result()
	if !hasResult {
		return session.Editability{}
	}
	return session.ComputeEditability(s.LastSQL, result.Columns, entry.Index, s.ReadOnly)
}

func (m Model) handleBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.browser.Update(msg)
	switch {
	case action.info != "":
		m, cmd := m.startTableInfo(action.info)
		return m, cmd
	case action.dbml:
		_, cmd := m.openDBML()
		return m, cmd
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

// View implements tea.Model.
func (m Model) View() string {
	lay := m.layout()

	var queryContent, resultsContent string
	if s := m.activeSession(); s != nil {
		queryContent = m.tabBar(max(lay.rightW-4, 1)) + "\n" + s.Editor.EditorView()
		resultsContent = s.Results.View()
	} else {
		queryContent = "no session"
		resultsContent = "no results yet"
	}

	queryTitle := "Query"
	if s := m.activeSession(); s != nil {
		queryTitle += " · " + s.Editor.ModeLabel()
	}
	queryView := m.pane(queryTitle, queryContent, lay.rightW, lay.queryH, m.focus == FocusQuery)
	results := m.pane("Results", resultsContent, lay.rightW, lay.resultH, m.focus == FocusResult)

	panes := []string{lipgloss.JoinVertical(lipgloss.Left, queryView, results)}
	if lay.workspaceW > 0 {
		workspaceView := m.pane("Explorer", m.workspace.View(), lay.workspaceW, lay.bodyH, m.focus == FocusWorkspace)
		panes = []string{workspaceView, panes[0]}
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, panes...)
	status := m.theme.Status.Width(lay.width).Render(m.statusLine(lay))
	helpView := m.theme.Help.Width(lay.width).Render(m.help.View(m.keys))

	view := lipgloss.JoinVertical(lipgloss.Left, body, status, helpView)
	if title, body, ok := m.workspace.Modal(); ok {
		view = m.renderModal(view, lay, modalBox{Title: title, Body: body})
	} else if m.export != nil {
		view = m.renderModal(view, lay, modalBox{Title: "Export results · " + m.sessionLabel(), Body: m.export.view()})
	} else if title, body, ok := m.favoritesDialog(); ok {
		view = m.renderModal(view, lay, modalBox{Title: title, Body: body})
	} else if m.browser != nil {
		view = m.renderModal(view, lay, modalBox{Title: m.browser.Title(), Body: m.browser.View()})
	} else if m.confirmClose != "" {
		view = m.renderModal(view, lay, modalBox{
			Title: "Close tab",
			Body:  "The query in this tab is running.\n\ny close · n cancel",
		})
	} else if m.helpOpen {
		view = m.renderModal(view, lay, modalBox{Title: "Keybindings", Body: helpText()})
	} else if m.menu != nil {
		view = m.renderModal(view, lay, modalBox{Title: "Command menu", Body: m.commandMenuView()})
	} else if m.doc != nil {
		view = m.renderDoc(view, lay)
	} else if m.cellEdit != nil {
		title := fmt.Sprintf("Edit cell · %s (row %d)", m.cellEdit.column, m.cellEdit.row+1)
		view = m.renderModal(view, lay, modalBox{Title: title, Body: m.cellEdit.view()})
	} else if m.rowMenu != nil {
		view = m.renderModal(view, lay, modalBox{Title: "Row actions", Body: m.rowMenu.view()})
	} else if m.preview != nil {
		view = m.renderModal(view, lay, modalBox{Title: "Preview changes", Body: m.preview.view()})
	} else if m.modal != nil {
		view = m.renderModal(view, lay, *m.modal)
	}
	return view
}

func (m Model) tabBar(width int) string {
	sessions := m.sessions.Sessions()
	active, _ := m.sessions.Active()
	parts := make([]string, 0, len(sessions)+1)
	for i, label := range m.tabLabels() {
		style := m.theme.PaneTitle
		if i < len(sessions) {
			style = m.theme.Status
			if s := sessions[i]; active != nil && s.ID == active.ID {
				style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
			}
		}
		parts = append(parts, style.Render(label))
	}
	return truncate(strings.Join(parts, m.theme.Status.Render(" │ ")), width)
}

// tabLabels returns the plain tab labels (last entry is the "+" affordance).
func (m Model) tabLabels() []string {
	sessions := m.sessions.Sessions()
	labels := make([]string, 0, len(sessions)+1)
	for i, s := range sessions {
		label := fmt.Sprintf("%d: ", i+1)
		if s.Running() {
			label += "● "
		}
		labels = append(labels, label+s.Title)
	}
	return append(labels, "+")
}

// sessionLabel is the tab name shown in per-session dialog titles.
func (m Model) sessionLabel() string {
	if s := m.activeSession(); s != nil {
		return s.Title
	}
	return "untitled"
}

// favoritesDialog adds the active tab name to favorites overlays.
func (m Model) favoritesDialog() (string, string, bool) {
	title, body, ok := m.favorites.Modal()
	if !ok {
		return "", "", false
	}
	return title + " · " + m.sessionLabel(), body, true
}

func (m Model) statusLine(lay layout) string {
	parts := []string{"sql-tui v" + Version, "focus: " + m.focus.String()}
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
	if s := m.activeSession(); s != nil {
		if _, hasResult := s.Results.Result(); hasResult {
			switch {
			case s.Editability.Editable:
				line += " · editable: PK " + strings.Join(s.Editability.PrimaryKey, ", ")
				if row, column, _, ok := s.Results.SelectedCell(); ok {
					line += fmt.Sprintf(" · cell: %s (row %d)", column, row+1)
				}
			case s.Editability.Reason != "":
				line += " · read-only (" + s.Editability.Reason + ")"
			}
		}
	}
	if m.notice != "" {
		line = m.notice + " · " + line
	}
	return truncate(line, lay.width)
}

// resize propagates the current pane geometry to child models. Pane content
// includes the title row, so children get height-3 (border and title removed).
func (m *Model) resize() {
	lay := m.layout()
	m.wsHidden = lay.workspaceW == 0
	if !m.wsHidden {
		m.workspace.SetSize(max(lay.workspaceW-4, 1), max(lay.bodyH-3, 1))
	}
	for _, s := range m.sessions.Sessions() {
		s.Editor.SetSize(max(lay.rightW-4, 1), max(lay.queryH-4, 1)) // tab bar row
		s.Results.SetSize(max(lay.rightW-4, 1), max(lay.resultH-3, 1))
	}
	m.favorites.SetSize(lay.width, lay.height)
	if m.browser != nil {
		m.browser.SetSize(lay.width, lay.height)
	}
	m.resizeDoc()
	if m.active != nil {
		m.workspace.SetActive(m.active.ID)
	}
	if m.wsHidden && m.focus == FocusWorkspace {
		m.focus = FocusQuery
		m.syncFocus()
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
	if ws > w-1 {
		ws = w / 2
	}
	if w < collapseWidth {
		ws = 0
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
	// Long errors/args can wrap; keep the modal inside the screen.
	rendered = limitLines(rendered, max(lay.height-2, 3))
	return overlay(m.dimLines(background), rendered, lay.width)
}

// limitLines clips a rendered block to maxLines, appending an ellipsis marker
// when content was dropped.
func limitLines(block string, maxLines int) string {
	lines := strings.Split(block, "\n")
	if len(lines) <= maxLines {
		return block
	}
	lines = lines[:maxLines]
	lines[maxLines-1] = ansi.Truncate(lines[maxLines-1], 200, "…") + "…"
	return strings.Join(lines, "\n")
}

func (m Model) dimLines(background string) string {
	lines := strings.Split(background, "\n")
	for i := range lines {
		lines[i] = m.theme.Dim.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}
