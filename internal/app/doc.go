package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/session"
)

// docViewer is a full-width, scrollable document overlay used for table info
// and DBML rendering.
type docViewer struct {
	title    string
	body     string
	viewport viewport.Model
}

func (m *Model) openDoc(title, body string) {
	lay := m.layout()
	v := viewport.New(max(lay.width-10, 10), max(lay.height-6, 3))
	v.SetContent(body)
	m.doc = &docViewer{title: title, body: body, viewport: v}
}

type docCopiedMsg struct {
	Err error
}

func (m Model) copyDoc() tea.Cmd {
	if m.doc == nil {
		return nil
	}
	body := m.doc.body
	clipboard := m.clipboard
	return func() tea.Msg {
		return docCopiedMsg{Err: clipboard.WriteAll(body)}
	}
}

func (m Model) handleDocKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.doc = nil
		return m, nil
	case "c":
		return m, m.copyDoc()
	}
	var cmd tea.Cmd
	m.doc.viewport, cmd = m.doc.viewport.Update(msg)
	return m, cmd
}

func (m Model) renderDoc(background string, lay layout) string {
	body := m.doc.viewport.View()
	box := m.theme.Modal.Width(lay.width - 6).Render(body)
	header := m.theme.ModalTitle.Render(m.doc.title)
	content := header + "\n\n" + box
	content = limitLines(content, max(lay.height-2, 3))
	return overlay(m.dimLines(background), content, lay.width)
}

func (m *Model) resizeDoc() {
	if m.doc == nil {
		return
	}
	lay := m.layout()
	m.doc.viewport.Width = max(lay.width-10, 10)
	m.doc.viewport.Height = max(lay.height-6, 3)
}

// startTableInfo fetches table metadata in a command and opens the viewer.
func (m Model) startTableInfo(table string) (tea.Model, tea.Cmd) {
	s := m.activeSession()
	if s == nil {
		cmd := m.setNotice("no active session")
		return m, cmd
	}
	connectionID := s.ConnectionID
	driver := s.Driver
	if connectionID == "" && m.active != nil {
		connectionID = m.active.ID
		driver = m.active.Driver
	}
	handle, ok := m.manager.Get(connectionID)
	if !ok {
		cmd := m.setNotice("connection is not open")
		return m, cmd
	}
	m.notice = "loading table info…"
	sessionID := s.ID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := db.GetTableInfo(ctx, handle, driver, table)
		return tableInfoDoneMsg{SessionID: sessionID, Table: table, Info: info, Err: err}
	}
}

type tableInfoDoneMsg struct {
	SessionID session.ID
	Table     string
	Info      *db.TableInfo
	Err       error
}

func (m Model) handleTableInfoDone(msg tableInfoDoneMsg) (tea.Model, tea.Cmd) {
	s, ok := m.sessions.Get(msg.SessionID)
	if !ok {
		return m, nil
	}
	_ = s
	if msg.Err != nil {
		m.openModal("Table info failed", msg.Err.Error())
		return m, nil
	}
	if msg.Info == nil {
		m.openModal("Table info failed", "no metadata returned")
		return m, nil
	}
	m.openDoc(fmt.Sprintf("Table info · %s", msg.Info.Table), formatTableInfo(msg.Info))
	return m, nil
}

func formatTableInfo(info *db.TableInfo) string {
	section := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	faint := lipgloss.NewStyle().Faint(true)
	var b strings.Builder

	b.WriteString(section.Render("Columns"))
	b.WriteString("\n")
	if len(info.Columns) == 0 {
		b.WriteString(faint.Render("  (none)") + "\n")
	}
	for _, column := range info.Columns {
		line := fmt.Sprintf("  %s %s", column.Name, column.Type)
		markers := make([]string, 0, 2)
		if column.IsPrimaryKey {
			markers = append(markers, "PK")
		}
		if !column.Nullable {
			markers = append(markers, "NOT NULL")
		}
		if len(markers) > 0 {
			line += faint.Render("  " + strings.Join(markers, ", "))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + section.Render("Foreign Keys") + "\n")
	if len(info.ForeignKeys) == 0 {
		b.WriteString(faint.Render("  (none)") + "\n")
	}
	for _, fk := range info.ForeignKeys {
		fmt.Fprintf(&b, "  %s → %s.%s\n", fk.Column, fk.RefTable, fk.RefColumn)
	}

	b.WriteString("\n" + section.Render("Indexes") + "\n")
	if len(info.Indexes) == 0 {
		b.WriteString(faint.Render("  (none)") + "\n")
	}
	for _, index := range info.Indexes {
		marker := ""
		if index.Unique {
			marker = faint.Render(" unique")
		}
		fmt.Fprintf(&b, "  %s%s: %s\n", index.Name, marker, strings.Join(index.Columns, ", "))
	}

	b.WriteString("\n" + section.Render("Constraints") + "\n")
	if len(info.Constraints) == 0 {
		b.WriteString(faint.Render("  (none)") + "\n")
	}
	for _, constraint := range info.Constraints {
		fmt.Fprintf(&b, "  %s %s: %s\n", constraint.Type, constraint.Name, strings.Join(constraint.Columns, ", "))
	}

	b.WriteString("\n" + section.Render("DDL") + "\n")
	b.WriteString(info.DDL)
	return strings.TrimRight(b.String(), "\n")
}

// openDBML renders the active connection's cached schema as DBML.
func (m *Model) openDBML() (tea.Model, tea.Cmd) {
	connectionID := ""
	if s := m.activeSession(); s != nil {
		connectionID = s.ConnectionID
	} else if m.active != nil {
		connectionID = m.active.ID
	}
	if connectionID == "" {
		cmd := m.setNotice("select a connection first")
		return *m, cmd
	}
	entry, ok := m.schema.Get(connectionID)
	if !ok || len(entry.Tables) == 0 {
		cmd := m.setNotice("schema not loaded yet")
		return *m, cmd
	}
	title := "DBML · " + connectionLabelName(m, connectionID)
	m.openDoc(title, db.DBML(entry.Tables))
	return *m, nil
}

func connectionLabelName(m *Model, connectionID string) string {
	if m.active != nil && m.active.ID == connectionID {
		return m.active.Name
	}
	if s := m.activeSession(); s != nil && s.ConnectionID == connectionID {
		return s.Title
	}
	return connectionID
}

var _ = ansi.Truncate // keep x/ansi available for future doc styling
