// Package favorites stores and browses named SQL scripts.
package favorites

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LoadMsg asks the root model to put a favorite's SQL into the editor.
type LoadMsg struct {
	Name string
	SQL  string
}

type loadedMsg struct {
	favorites []Favorite
	err       error
}

type savedMsg struct {
	favorites []Favorite
	err       error
}

type deletedMsg struct {
	favorites []Favorite
	err       error
}

type saveForm struct {
	name textinput.Model
	sql  string
	err  string
}

func newSaveForm(sql string) *saveForm {
	name := textinput.New()
	name.Placeholder = "favorite name"
	name.Prompt = ""
	name.CharLimit = 80
	name.Width = 40
	name.Focus()

	return &saveForm{name: name, sql: sql}
}

func (f *saveForm) update(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEnter {
		return true
	}
	var cmd tea.Cmd
	f.name, cmd = f.name.Update(msg)
	_ = cmd
	return false
}

func (f *saveForm) view() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Name"))
	b.WriteString("\n")
	b.WriteString(f.name.View())
	if f.err != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(f.err))
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("enter save · esc cancel"))
	return b.String()
}

type overwritePrompt struct {
	name string
	sql  string
}

type deleteConfirm struct {
	favorite Favorite
}

// Model is the favorites list and its dialogs. The root model routes keys and
// renders Modal() as an overlay.
type Model struct {
	store *Store
	list  list.Model

	favorites []Favorite
	loaded    bool
	visible   bool

	save      *saveForm
	overwrite *overwritePrompt
	confirm   *deleteConfirm
	modalErr  error

	width, height int
}

// New returns a favorites model. A nil store keeps changes in memory only.
func New(store *Store) Model {
	l := list.New(nil, list.NewDefaultDelegate(), 50, 10)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	return Model{store: store, list: l}
}

// Init loads persisted favorites.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return loadedMsg{}
		}
		favorites, err := m.store.Load()
		return loadedMsg{favorites: favorites, err: err}
	}
}

// SetSize records the screen size so the overlay can size its list.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.resizeList()
}

func (m *Model) resizeList() {
	width := clamp(m.width-14, 20, 50)
	height := clamp(m.height-12, 3, 10)
	m.list.SetSize(width, height)
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

// OpenBrowse shows the favorites overlay.
func (m *Model) OpenBrowse() {
	m.visible = true
	m.modalErr = nil
	m.resizeList()
	m.list.SetItems(items(m.favorites))
	m.list.ResetFilter()
	m.list.ResetSelected()
}

// OpenSave shows the save dialog for the given SQL.
func (m *Model) OpenSave(sql string) {
	m.visible = false
	m.confirm = nil
	m.overwrite = nil
	m.save = newSaveForm(sql)
}

// CapturesInput reports whether the overlay owns the keyboard.
func (m Model) CapturesInput() bool {
	return m.visible || m.save != nil || m.overwrite != nil || m.confirm != nil || m.modalErr != nil
}

// Modal describes the active overlay for the root model to centre.
func (m Model) Modal() (title, body string, ok bool) {
	switch {
	case m.modalErr != nil:
		return "Favorites error", m.modalErr.Error() + "\n\nesc to close", true
	case m.overwrite != nil:
		return "Overwrite favorite",
			fmt.Sprintf("A favorite named %q already exists.\n\ny overwrite · n cancel", m.overwrite.name), true
	case m.confirm != nil:
		return "Delete favorite",
			fmt.Sprintf("Delete %q?\n\ny confirm · n cancel", m.confirm.favorite.Name), true
	case m.save != nil:
		return "Save favorite", m.save.view(), true
	case m.visible:
		return "Favorites", m.browseBody(), true
	default:
		return "", "", false
	}
}

func (m Model) browseBody() string {
	if !m.loaded {
		return "loading favorites…"
	}
	if len(m.favorites) == 0 {
		return "no favorites yet\n\nctrl+s in the query pane to save one\n\nesc close"
	}
	return m.list.View()
}

// Update handles messages and keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.loaded = true
		if msg.err != nil {
			m.modalErr = msg.err
			return m, nil
		}
		m.favorites = msg.favorites
		m.list.SetItems(items(m.favorites))
		return m, nil
	case savedMsg:
		m.favorites = msg.favorites
		m.list.SetItems(items(m.favorites))
		if msg.err != nil {
			m.modalErr = msg.err
		}
		return m, nil
	case deletedMsg:
		m.favorites = msg.favorites
		m.list.SetItems(items(m.favorites))
		if msg.err != nil {
			m.modalErr = msg.err
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.modalErr != nil {
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "enter"))) {
			m.modalErr = nil
		}
		return m, nil
	}

	if m.overwrite != nil {
		switch msg.String() {
		case "y", "Y":
			return m.applyOverwrite()
		case "n", "N", "esc":
			m.overwrite = nil
		}
		return m, nil
	}

	if m.confirm != nil {
		switch msg.String() {
		case "y", "Y":
			return m.deleteConfirmed()
		case "n", "N", "esc":
			m.confirm = nil
		}
		return m, nil
	}

	if m.save != nil {
		if msg.Type == tea.KeyEsc {
			m.save = nil
			return m, nil
		}
		if m.save.update(msg) {
			return m.submitSave()
		}
		return m, nil
	}

	if !m.visible {
		return m, nil
	}

	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "esc":
		m.visible = false
		return m, nil
	case "enter":
		favorite, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.visible = false
		loaded := favorite
		return m, func() tea.Msg { return LoadMsg{Name: loaded.Name, SQL: loaded.SQL} }
	case "d":
		favorite, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.confirm = &deleteConfirm{favorite: favorite}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) submitSave() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.save.name.Value())
	if name == "" {
		m.save.err = "name is required"
		return m, nil
	}
	for _, existing := range m.favorites {
		if strings.EqualFold(existing.Name, name) {
			m.overwrite = &overwritePrompt{name: name, sql: m.save.sql}
			return m, nil
		}
	}
	return m.persistNew(name, m.save.sql)
}

func (m Model) persistNew(name, sql string) (Model, tea.Cmd) {
	next := make([]Favorite, len(m.favorites))
	copy(next, m.favorites)
	next = append(next, Favorite{
		ID:        newID(),
		Name:      name,
		SQL:       sql,
		CreatedAt: time.Now(),
	})
	m.save = nil
	m.overwrite = nil
	m.visible = false
	return m, m.saveCmd(next)
}

func (m Model) applyOverwrite() (Model, tea.Cmd) {
	next := make([]Favorite, len(m.favorites))
	copy(next, m.favorites)
	for i, existing := range next {
		if strings.EqualFold(existing.Name, m.overwrite.name) {
			next[i].Name = m.overwrite.name
			next[i].SQL = m.overwrite.sql
		}
	}
	m.save = nil
	m.overwrite = nil
	m.visible = false
	return m, m.saveCmd(next)
}

func (m Model) deleteConfirmed() (Model, tea.Cmd) {
	id := m.confirm.favorite.ID
	m.confirm = nil
	next := make([]Favorite, 0, len(m.favorites))
	for _, existing := range m.favorites {
		if existing.ID != id {
			next = append(next, existing)
		}
	}
	return m, func() tea.Msg {
		var err error
		if m.store != nil {
			err = m.store.Save(next)
		}
		return deletedMsg{favorites: next, err: err}
	}
}

func (m Model) saveCmd(next []Favorite) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		var err error
		if store != nil {
			err = store.Save(next)
		}
		return savedMsg{favorites: next, err: err}
	}
}

func (m Model) selected() (Favorite, bool) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return Favorite{}, false
	}
	entry, ok := selected.(item)
	if !ok {
		return Favorite{}, false
	}
	return entry.favorite, true
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fav-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
