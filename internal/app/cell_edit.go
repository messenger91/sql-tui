package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/export"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/session"
)

// cellEditor is the overlay for editing one result cell (§12.2).
type cellEditor struct {
	row      int
	column   string
	meta     db.ColumnMeta
	original any
	wasNull  bool

	isNull bool
	input  textinput.Model
	err    string
}

func newCellEditor(row int, column string, value any, meta db.ColumnMeta) *cellEditor {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 512
	input.Width = 40
	if value != nil {
		input.SetValue(export.FormatValue(value))
	}
	input.Focus()

	return &cellEditor{
		row:      row,
		column:   column,
		meta:     meta,
		original: value,
		wasNull:  value == nil,
		isNull:   value == nil,
		input:    input,
	}
}

// update handles one key and reports whether the editor should submit.
func (e *cellEditor) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+n":
		if !e.meta.Nullable {
			e.err = "column is not nullable"
			return false
		}
		e.isNull = !e.isNull
		e.err = ""
		if e.isNull {
			e.input.Blur()
		} else {
			e.input.Focus()
		}
		return false
	}

	if msg.Type == tea.KeyEnter {
		return true
	}
	if e.isNull && msg.Type == tea.KeyRunes {
		e.isNull = false
		e.input.Focus()
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	_ = cmd
	return false
}

func (e *cellEditor) validate() (session.CellEdit, bool) {
	if e.isNull {
		return session.CellEdit{
			Value:        nil,
			IsNull:       true,
			Original:     e.original,
			OriginalNull: e.wasNull,
		}, true
	}
	value, err := session.ParseCellValue(e.meta, e.input.Value())
	if err != nil {
		e.err = err.Error()
		return session.CellEdit{}, false
	}
	return session.CellEdit{
		Value:        value,
		Original:     e.original,
		OriginalNull: e.wasNull,
	}, true
}

func (e *cellEditor) view() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(e.column))
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(e.meta.Type))
	if !e.meta.Nullable {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  NOT NULL"))
	}
	b.WriteString("\n\n")
	if e.isNull {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("NULL"))
	} else {
		b.WriteString(e.input.View())
	}
	if e.err != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(e.err))
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("enter save · ctrl+n NULL · esc cancel"))
	return b.String()
}

// rowMenu is the `m` overlay for row-level pending operations.
type rowMenu struct {
	row int
	err string
}

func (r *rowMenu) view() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Row %d\n\n", r.row+1))
	b.WriteString("n  set cell to NULL\n")
	b.WriteString("d  duplicate row (insert)\n")
	b.WriteString("x  delete row\n")
	if r.err != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(r.err))
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("esc close"))
	return b.String()
}

func (m *Model) syncPending(s *session.Session) {
	if s == nil || s.Pending == nil {
		return
	}
	s.Results.SetPending(s.Pending.RowSet(), s.Pending.Count())
}

// snapshotRow records the loaded row before its first change (§12.4 conflict
// detection).
func (m *Model) snapshotRow(s *session.Session, row int) {
	if s == nil || s.Pending == nil {
		return
	}
	data, ok := s.Results.Result()
	if !ok || row < 0 || row >= len(data.Rows) {
		return
	}
	values := make([]result.ColumnValue, len(data.Columns))
	for i, column := range data.Columns {
		var value any
		if i < len(data.Rows[row]) {
			value = data.Rows[row][i]
		}
		values[i] = result.ColumnValue{Column: column, Value: value}
	}
	s.Pending.EnsureSnapshot(row, values)
}

func (m *Model) beginCellEdit(s *session.Session) bool {
	row, column, value, ok := s.Results.SelectedCell()
	if !ok {
		return false
	}
	meta, ok := s.Editability.Columns[column]
	if !ok {
		return false
	}
	m.cellEdit = newCellEditor(row, column, value, meta)
	return true
}

func (m Model) handleCellEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.cellEdit = nil
		return m, nil
	}
	if m.cellEdit.update(msg) {
		edit, ok := m.cellEdit.validate()
		if !ok {
			return m, nil
		}
		if s := m.activeSession(); s != nil {
			m.snapshotRow(s, m.cellEdit.row)
			s.Pending.SetCell(m.cellEdit.row, m.cellEdit.column, edit)
			m.syncPending(s)
		}
		m.cellEdit = nil
	}
	return m, nil
}

func (m Model) handleRowMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.rowMenu = nil
		return m, nil
	case "n":
		s := m.activeSession()
		if s == nil {
			return m, nil
		}
		row, column, value, ok := s.Results.SelectedCell()
		if !ok {
			return m, nil
		}
		meta, ok := s.Editability.Columns[column]
		if !ok {
			return m, nil
		}
		if !meta.Nullable {
			m.rowMenu.err = "column is not nullable"
			return m, nil
		}
		m.snapshotRow(s, row)
		s.Pending.SetCell(row, column, session.CellEdit{
			Value:        nil,
			IsNull:       true,
			Original:     value,
			OriginalNull: value == nil,
		})
		m.syncPending(s)
		m.rowMenu = nil
		return m, nil
	case "d":
		if s := m.activeSession(); s != nil {
			m.snapshotRow(s, m.rowMenu.row)
			s.Pending.SetRowOp(m.rowMenu.row, session.RowDuplicate)
			m.syncPending(s)
		}
		m.rowMenu = nil
		return m, nil
	case "x":
		if s := m.activeSession(); s != nil {
			m.snapshotRow(s, m.rowMenu.row)
			s.Pending.SetRowOp(m.rowMenu.row, session.RowDelete)
			m.syncPending(s)
		}
		m.rowMenu = nil
		return m, nil
	}
	return m, nil
}
