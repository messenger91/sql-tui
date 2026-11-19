// Package app owns the root Bubble Tea model: the three-pane layout, focus
// routing, global keys, modals, and (from §11) the session message router.
package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/messenger91/sql-tui/internal/styles"
)

const (
	defaultWidth      = 80
	defaultHeight     = 24
	workspaceWidth    = 30
	minWorkspaceWidth = 12
	minBodyHeight     = 3
)

type modalBox struct {
	Title string
	Body  string
}

// Model is the root application model.
type Model struct {
	theme styles.Theme
	help  help.Model
	keys  keyMap

	focus         Focus
	width, height int
	modal         *modalBox
}

// New returns the root model focused on the workspace pane.
func New() Model {
	h := help.New()
	h.Width = defaultWidth
	return Model{
		theme: styles.Default(),
		help:  h,
		keys:  defaultKeyMap(),
		focus: FocusWorkspace,
	}
}

// Init implements tea.Model. Connection loading arrives with §4.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case sessionEventMsg:
		return m.routeSessionEvent(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
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
	case key.Matches(msg, m.keys.CyclePrev):
		m.focus = m.focus.Prev()
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	}
	return m, nil
}

// routeSessionEvent dispatches an async query event to its owning session.
// Sessions are introduced in §11; until then events are dropped.
func (m Model) routeSessionEvent(sessionEventMsg) (tea.Model, tea.Cmd) {
	return m, nil
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

	workspace := m.pane("Connections", "no connections yet", lay.workspaceW, lay.bodyH, m.focus == FocusWorkspace)
	query := m.pane("Query", "no connection selected", lay.rightW, lay.queryH, m.focus == FocusQuery)
	results := m.pane("Results", "no results yet", lay.rightW, lay.resultH, m.focus == FocusResult)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		workspace,
		lipgloss.JoinVertical(lipgloss.Left, query, results),
	)
	status := m.theme.Status.Width(lay.width).Render(
		fmt.Sprintf("sql-tui · focus: %s · %d×%d", m.focus, lay.width, lay.height),
	)
	helpView := m.theme.Help.Width(lay.width).Render(m.help.View(m.keys))

	view := lipgloss.JoinVertical(lipgloss.Left, body, status, helpView)
	if m.modal != nil {
		view = m.renderModal(view, lay)
	}
	return view
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

func (m Model) renderModal(background string, lay layout) string {
	boxWidth := min(56, max(lay.width-8, 8))
	content := m.theme.ModalTitle.Render(m.modal.Title) + "\n\n" + m.modal.Body
	box := m.theme.Modal.Width(boxWidth).Render(content)

	lines := m.dimLines(background)
	return overlay(lines, box, lay.width)
}

func (m Model) dimLines(background string) string {
	lines := strings.Split(background, "\n")
	for i := range lines {
		lines[i] = m.theme.Dim.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}
