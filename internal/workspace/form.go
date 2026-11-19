package workspace

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/messenger91/sql-tui/internal/db"
)

type formMode int

const (
	formAdd formMode = iota
	formEdit
)

// driverOrder is the cycle order for the driver selector.
var driverOrder = []db.Driver{db.DriverSQLite, db.DriverMySQL, db.DriverPostgres}

type form struct {
	mode     formMode
	id       string
	readOnly bool

	name   textinput.Model
	dsn    textinput.Model
	schema textinput.Model
	driver db.Driver
	focus  int // 0 name, 1 driver, 2 dsn, 3 schema, 4 access
	err    string
	width  int
}

func newForm(mode formMode) *form {
	name := textinput.New()
	name.Placeholder = "my database"
	name.CharLimit = 80
	name.Prompt = ""

	dsn := textinput.New()
	dsn.CharLimit = 512
	dsn.Prompt = ""

	schema := textinput.New()
	schema.CharLimit = 80
	schema.Prompt = ""
	schema.Placeholder = "optional schema / search_path"

	f := &form{
		mode:     mode,
		readOnly: true,
		name:     name,
		dsn:      dsn,
		schema:   schema,
		driver:   db.DriverSQLite,
		width:    50,
	}
	f.applyDriverHint()
	f.syncFocus()
	return f
}

func (f *form) setConnection(conn db.Connection) {
	f.mode = formEdit
	f.id = conn.ID
	f.readOnly = conn.ReadOnly
	f.driver = conn.Driver
	f.name.SetValue(conn.Name)
	f.dsn.SetValue(conn.DSN)
	if conn.Options != nil {
		f.schema.SetValue(conn.Options[db.OptionSchema])
	}
	f.applyDriverHint()
	f.syncFocus()
}

func (f *form) setWidth(width int) {
	f.width = max(width, 24)
}

func (f *form) syncFocus() {
	f.name.Blur()
	f.dsn.Blur()
	switch f.focus {
	case 0:
		f.name.Focus()
	case 2:
		f.dsn.Focus()
	case 3:
		f.schema.Focus()
	}
}

func (f *form) applyDriverHint() {
	f.dsn.Placeholder, _ = driverHint(f.driver)
}

func driverHint(driver db.Driver) (string, string) {
	switch driver {
	case db.DriverMySQL:
		return "user:pass@tcp(127.0.0.1:3306)/dbname", "MySQL DSN, e.g. user:pass@tcp(host:3306)/db"
	case db.DriverPostgres:
		return "postgres://user:pass@localhost:5432/dbname?sslmode=disable", "Postgres URL, e.g. postgres://user@host:5432/db"
	default:
		return ":memory:", "SQLite file path, or :memory: for an in-memory database"
	}
}

func (f *form) cycleDriver(delta int) {
	current := 0
	for i, driver := range driverOrder {
		if driver == f.driver {
			current = i
			break
		}
	}
	next := (current + delta + len(driverOrder)) % len(driverOrder)
	f.driver = driverOrder[next]
	f.applyDriverHint()
}

// update handles one key press. It reports true when the form should submit.
func (f *form) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab":
		f.focus = (f.focus + 1) % 5
		f.syncFocus()
		return false
	case "shift+tab":
		f.focus = (f.focus + 4) % 5
		f.syncFocus()
		return false
	}

	if f.focus == 1 {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			f.cycleDriver(-1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			f.cycleDriver(1)
		case msg.Type == tea.KeyEnter:
			return true
		}
		return false
	}

	if f.focus == 4 {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "right", "h", "l", " "))):
			f.readOnly = !f.readOnly
		case msg.Type == tea.KeyEnter:
			return true
		}
		return false
	}

	if msg.Type == tea.KeyEnter {
		return true
	}
	var cmd tea.Cmd
	switch f.focus {
	case 0:
		f.name, cmd = f.name.Update(msg)
	case 2:
		f.dsn, cmd = f.dsn.Update(msg)
	case 3:
		f.schema, cmd = f.schema.Update(msg)
	}
	_ = cmd // cursor blink is not needed for a transient modal
	return false
}

func (f *form) validate() (db.Connection, bool) {
	name := strings.TrimSpace(f.name.Value())
	dsn := strings.TrimSpace(f.dsn.Value())
	switch {
	case name == "":
		f.err = "name is required"
		return db.Connection{}, false
	case !f.driver.Valid():
		f.err = "choose a valid driver"
		return db.Connection{}, false
	case dsn == "":
		f.err = "DSN is required"
		return db.Connection{}, false
	}
	schema := strings.TrimSpace(f.schema.Value())
	if schema != "" && f.driver == db.DriverSQLite {
		f.err = "schema is not supported for SQLite"
		return db.Connection{}, false
	}
	var options map[string]string
	if schema != "" {
		options = map[string]string{db.OptionSchema: schema}
	}
	f.err = ""
	return db.Connection{
		ID:       f.id,
		Name:     name,
		Driver:   f.driver,
		DSN:      dsn,
		Options:  options,
		ReadOnly: f.readOnly,
	}, true
}

func (f *form) title() string {
	if f.mode == formEdit {
		return "Edit connection"
	}
	return "Add connection"
}

func (f *form) view(width int) string {
	inputWidth := max(min(width-4, 60), 16)
	f.name.Width = inputWidth
	f.dsn.Width = inputWidth
	f.schema.Width = inputWidth

	label := lipgloss.NewStyle().Bold(true)
	var b strings.Builder
	b.WriteString(label.Render("Name"))
	b.WriteString("\n")
	b.WriteString(f.name.View())
	b.WriteString("\n\n")
	b.WriteString(label.Render("Driver"))
	b.WriteString("\n◀ ")
	b.WriteString(string(f.driver))
	b.WriteString(" ▶")
	b.WriteString("\n\n")
	b.WriteString(label.Render("DSN"))
	b.WriteString("\n")
	b.WriteString(f.dsn.View())
	b.WriteString("\n\n")
	b.WriteString(label.Render("Schema"))
	b.WriteString("\n")
	b.WriteString(f.schema.View())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("optional: Postgres search_path / MySQL database"))
	b.WriteString("\n\n")
	b.WriteString(label.Render("Access"))
	b.WriteString("\n◀ ")
	if f.readOnly {
		b.WriteString("read-only")
	} else {
		b.WriteString("writable")
	}
	b.WriteString(" ▶")

	placeholder, hint := driverHint(f.driver)
	_ = placeholder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(hint))

	if f.err != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(f.err))
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("tab next field · ←/→ change · enter connect · esc cancel"))
	return b.String()
}
