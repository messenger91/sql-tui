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

	"github.com/messenger91/sql-tui/internal/db"
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

// Config carries filesystem locations used by the app. The zero value keeps
// everything in memory, which is what tests want.
type Config struct {
	ConnectionsPath string
}

// Model is the root application model.
type Model struct {
	theme     styles.Theme
	help      help.Model
	keys      keyMap
	manager   *db.Manager
	workspace workspace.Model

	focus         Focus
	width, height int
	modal         *modalBox

	active    *db.Connection
	schema    []db.TableMeta
	schemaErr error
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
	return Model{
		theme:     styles.Default(),
		help:      h,
		keys:      defaultKeyMap(),
		manager:   manager,
		workspace: workspace.New(store, manager),
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
	return m.workspace.Init()
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
	case sessionEventMsg:
		return m.routeSessionEvent(msg)
	default:
		var cmd tea.Cmd
		m.workspace, cmd = m.workspace.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
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
		return m, nil
	case key.Matches(msg, m.keys.CyclePrev):
		m.focus = m.focus.Prev()
		return m, nil
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.resize()
		return m, nil
	}
	if m.focus == FocusWorkspace {
		var cmd tea.Cmd
		m.workspace, cmd = m.workspace.Update(msg)
		return m, cmd
	}
	return m, nil
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
	query := m.pane("Query", "no connection selected", lay.rightW, lay.queryH, m.focus == FocusQuery)
	results := m.pane("Results", "no results yet", lay.rightW, lay.resultH, m.focus == FocusResult)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		workspaceView,
		lipgloss.JoinVertical(lipgloss.Left, query, results),
	)
	status := m.theme.Status.Width(lay.width).Render(m.statusLine(lay))
	helpView := m.theme.Help.Width(lay.width).Render(m.help.View(m.keys))

	view := lipgloss.JoinVertical(lipgloss.Left, body, status, helpView)
	if title, body, ok := m.workspace.Modal(); ok {
		view = m.renderModal(view, lay, modalBox{Title: title, Body: body})
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
	return strings.Join(parts, " · ")
}

// resize propagates the current pane geometry to child models.
func (m *Model) resize() {
	lay := m.layout()
	m.workspace.SetSize(max(lay.workspaceW-4, 1), max(lay.bodyH-2, 1))
	if m.active != nil {
		m.workspace.SetActive(m.active.ID)
	}
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
