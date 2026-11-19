package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/messenger91/sql-tui/internal/db"
)

type browserKind int

const (
	browserTable browserKind = iota
	browserView
	browserColumn
)

type browserRow struct {
	kind       browserKind
	name       string
	table      string
	typeName   string
	primaryKey bool
	foreignKey string
}

func (r browserRow) identifier() string {
	return r.name
}

type browserAction struct {
	close   bool
	refresh bool
	insert  string
}

// schemaBrowser is the ctrl+b overlay: an expandable schema tree with fuzzy
// filtering and identifier insertion.
type schemaBrowser struct {
	tables     []db.TableMeta
	schemaName string

	expanded  map[string]bool
	filter    textinput.Model
	filtering bool

	rows   []browserRow
	cursor int
	offset int

	bodyWidth  int
	bodyHeight int
}

func newSchemaBrowser() *schemaBrowser {
	filter := textinput.New()
	filter.Prompt = ""
	filter.Placeholder = "filter tables and columns"
	filter.Width = 40
	filter.CharLimit = 80

	return &schemaBrowser{
		expanded:   make(map[string]bool),
		filter:     filter,
		bodyWidth:  50,
		bodyHeight: 12,
	}
}

// Open resets the browser for a new schema snapshot.
func (b *schemaBrowser) Open(tables []db.TableMeta) {
	b.tables = tables
	b.expanded = make(map[string]bool)
	b.filter.SetValue("")
	b.filtering = false
	b.cursor = 0
	b.offset = 0
	b.schemaName = ""
	for _, table := range tables {
		if table.Schema != "" {
			b.schemaName = table.Schema
			break
		}
	}
	b.rebuild()
}

// SetTables refreshes the snapshot while preserving expansion and filter.
func (b *schemaBrowser) SetTables(tables []db.TableMeta) {
	b.tables = tables
	for _, table := range tables {
		if table.Schema != "" {
			b.schemaName = table.Schema
			break
		}
	}
	b.rebuild()
}

// SetSize records the screen size so the modal body fits.
func (b *schemaBrowser) SetSize(width, height int) {
	b.bodyWidth = clamp(width-14, 20, 50)
	b.bodyHeight = clamp(height-12, 4, 14)
}

func (b *schemaBrowser) Title() string {
	if b.schemaName == "" {
		return "Schema"
	}
	return "Schema · " + b.schemaName
}

func (b *schemaBrowser) rebuild() {
	selected, hadSelection := b.selected()
	b.rows = b.rows[:0]

	if query := strings.TrimSpace(b.filter.Value()); query != "" {
		b.rows = b.filteredRows(query)
	} else {
		for _, table := range b.tables {
			kind := browserTable
			if table.Kind == db.TableKindView {
				kind = browserView
			}
			b.rows = append(b.rows, browserRow{kind: kind, name: table.Name})
			if !b.expanded[table.Name] {
				continue
			}
			for _, column := range table.Columns {
				row := browserRow{
					kind:       browserColumn,
					name:       column.Name,
					table:      table.Name,
					typeName:   column.Type,
					primaryKey: column.IsPrimaryKey,
				}
				for _, fk := range table.ForeignKeys {
					if fk.Column == column.Name {
						row.foreignKey = fk.RefTable + "." + fk.RefColumn
						break
					}
				}
				b.rows = append(b.rows, row)
			}
		}
	}

	if hadSelection {
		for i, row := range b.rows {
			if row.kind == selected.kind && row.name == selected.name && row.table == selected.table {
				b.cursor = i
				b.clampOffset()
				return
			}
		}
	}
	b.cursor = clamp(b.cursor, 0, max(len(b.rows)-1, 0))
	b.clampOffset()
}

func (b *schemaBrowser) filteredRows(query string) []browserRow {
	var candidates []browserRow
	var labels []string
	for _, table := range b.tables {
		kind := browserTable
		if table.Kind == db.TableKindView {
			kind = browserView
		}
		candidates = append(candidates, browserRow{kind: kind, name: table.Name})
		labels = append(labels, table.Name)
		for _, column := range table.Columns {
			row := browserRow{
				kind:       browserColumn,
				name:       column.Name,
				table:      table.Name,
				typeName:   column.Type,
				primaryKey: column.IsPrimaryKey,
			}
			for _, fk := range table.ForeignKeys {
				if fk.Column == column.Name {
					row.foreignKey = fk.RefTable + "." + fk.RefColumn
					break
				}
			}
			candidates = append(candidates, row)
			labels = append(labels, column.Name)
		}
	}

	matches := fuzzy.Find(query, labels)
	rows := make([]browserRow, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, candidates[match.Index])
	}
	return rows
}

func (b *schemaBrowser) selected() (browserRow, bool) {
	if b.cursor < 0 || b.cursor >= len(b.rows) {
		return browserRow{}, false
	}
	return b.rows[b.cursor], true
}

func (b *schemaBrowser) move(delta int) {
	b.cursor = clamp(b.cursor+delta, 0, max(len(b.rows)-1, 0))
	b.clampOffset()
}

func (b *schemaBrowser) clampOffset() {
	if b.bodyHeight <= 0 {
		return
	}
	if b.cursor < b.offset {
		b.offset = b.cursor
	}
	if b.cursor >= b.offset+b.bodyHeight {
		b.offset = b.cursor - b.bodyHeight + 1
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

// Update handles a key and reports what the root model should do.
func (b *schemaBrowser) Update(msg tea.KeyMsg) browserAction {
	if b.filtering {
		switch msg.String() {
		case "esc":
			b.filtering = false
			b.filter.Blur()
			return browserAction{}
		case "up", "ctrl+p":
			b.move(-1)
			return browserAction{}
		case "down", "ctrl+n":
			b.move(1)
			return browserAction{}
		case "enter":
			if row, ok := b.selected(); ok {
				return browserAction{close: true, insert: row.identifier()}
			}
			return browserAction{}
		}
		var cmd tea.Cmd
		b.filter, cmd = b.filter.Update(msg)
		_ = cmd
		b.rebuild()
		return browserAction{}
	}

	switch msg.String() {
	case "esc":
		return browserAction{close: true}
	case "ctrl+r":
		return browserAction{refresh: true}
	case "up", "k":
		b.move(-1)
	case "down", "j":
		b.move(1)
	case "pgup":
		b.move(-b.bodyHeight)
	case "pgdown":
		b.move(b.bodyHeight)
	case "home", "g":
		b.cursor = 0
		b.clampOffset()
	case "end", "G":
		b.cursor = max(len(b.rows)-1, 0)
		b.clampOffset()
	case "right", "l":
		b.expand()
	case "left", "h":
		b.collapse()
	case "enter":
		if row, ok := b.selected(); ok {
			return browserAction{close: true, insert: row.identifier()}
		}
	case "/":
		b.filtering = true
		b.filter.SetValue("")
		b.filter.Focus()
		b.rebuild()
	}
	return browserAction{}
}

func (b *schemaBrowser) expand() {
	row, ok := b.selected()
	if !ok || row.kind == browserColumn || b.expanded[row.name] {
		return
	}
	b.expanded[row.name] = true
	b.rebuild()
}

func (b *schemaBrowser) collapse() {
	row, ok := b.selected()
	if !ok {
		return
	}
	if row.kind == browserColumn {
		for i := b.cursor - 1; i >= 0; i-- {
			if b.rows[i].kind != browserColumn && b.rows[i].name == row.table {
				b.cursor = i
				b.clampOffset()
				return
			}
		}
		return
	}
	if !b.expanded[row.name] {
		return
	}
	delete(b.expanded, row.name)
	b.rebuild()
}

// View renders the tree body, scroll window, filter input, and key hints.
func (b *schemaBrowser) View() string {
	var lines []string
	if len(b.tables) == 0 {
		lines = append(lines, "no tables found")
	} else if len(b.rows) == 0 {
		lines = append(lines, "no matches")
	} else {
		end := min(b.offset+b.bodyHeight, len(b.rows))
		for i := b.offset; i < end; i++ {
			lines = append(lines, b.rowView(b.rows[i], i))
		}
	}

	if b.filtering {
		lines = append(lines, b.filter.View())
	}
	lines = append(lines,
		lipgloss.NewStyle().Faint(true).Render(
			ansi.Truncate("↑/↓ move · →/← expand · enter insert · / filter · ctrl+r refresh · esc close", b.bodyWidth, "…"),
		),
	)
	return strings.Join(lines, "\n")
}

func (b *schemaBrowser) rowView(row browserRow, index int) string {
	dim := lipgloss.NewStyle().Faint(true)
	marker := "  "
	switch row.kind {
	case browserTable, browserView:
		if b.expanded[row.name] {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
	}

	var line strings.Builder
	if row.kind == browserColumn {
		line.WriteString("    ")
	} else {
		line.WriteString(marker)
	}
	line.WriteString(row.name)

	switch row.kind {
	case browserView:
		line.WriteString(dim.Render(" (view)"))
	case browserColumn:
		if row.typeName != "" {
			line.WriteString(dim.Render("  " + row.typeName))
		}
		if row.primaryKey {
			line.WriteString(dim.Render("  PK"))
		}
		if row.foreignKey != "" {
			line.WriteString(dim.Render("  → " + row.foreignKey))
		}
		if b.filter.Value() != "" {
			line.WriteString(dim.Render("  (" + row.table + ")"))
		}
	}

	lineText := ansi.Truncate(line.String(), b.bodyWidth, "…")
	if index == b.cursor {
		return lipgloss.NewStyle().Bold(true).Reverse(true).Render(lineText)
	}
	return lineText
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
