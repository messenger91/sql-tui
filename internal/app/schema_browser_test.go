package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

func browserTables() []db.TableMeta {
	return []db.TableMeta{
		{
			Schema: "main", Name: "users", Kind: db.TableKindTable,
			Columns: []db.ColumnMeta{
				{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
				{Name: "email", Type: "TEXT", Nullable: true},
			},
		},
		{
			Schema: "main", Name: "orders", Kind: db.TableKindTable,
			Columns: []db.ColumnMeta{
				{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
				{Name: "user_id", Type: "INTEGER"},
			},
			ForeignKeys: []db.ForeignKey{{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		},
		{
			Schema: "main", Name: "active_users", Kind: db.TableKindView,
			Columns: []db.ColumnMeta{{Name: "id", Type: "INTEGER"}},
		},
	}
}

func openTestBrowser(t *testing.T) *schemaBrowser {
	t.Helper()
	browser := newSchemaBrowser()
	browser.SetSize(80, 24)
	browser.Open(browserTables())
	return browser
}

func pressBrowser(browser *schemaBrowser, key tea.KeyMsg) browserAction {
	return browser.Update(key)
}

func TestBrowserTreeAndExpansion(t *testing.T) {
	browser := openTestBrowser(t)
	if len(browser.rows) != 3 {
		t.Fatalf("rows = %+v", browser.rows)
	}
	if view := ansi.Strip(browser.View()); !strings.Contains(view, "users") || !strings.Contains(view, "(view)") {
		t.Fatalf("view:\n%s", view)
	}

	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRight})
	if len(browser.rows) != 5 {
		t.Fatalf("rows after expand = %d", len(browser.rows))
	}
	view := ansi.Strip(browser.View())
	for _, want := range []string{"email", "PK"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded view missing %q:\n%s", want, view)
		}
	}

	// Move to the orders table and expand it to see the FK hint.
	browser.cursor = 3
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRight})
	view = ansi.Strip(browser.View())
	if !strings.Contains(view, "→ users.id") {
		t.Fatalf("FK hint missing:\n%s", view)
	}
}

func TestBrowserCollapseReturnsToTable(t *testing.T) {
	browser := openTestBrowser(t)
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRight}) // expand users
	browser.cursor = 1                                    // id column
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyLeft})
	if browser.cursor != 0 {
		t.Fatalf("cursor = %d", browser.cursor)
	}
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyLeft}) // collapse users
	if len(browser.rows) != 3 {
		t.Fatalf("rows = %d", len(browser.rows))
	}
}

func TestBrowserEnterInsertsIdentifier(t *testing.T) {
	browser := openTestBrowser(t)
	action := pressBrowser(browser, tea.KeyMsg{Type: tea.KeyEnter})
	if !action.close || action.insert != "users" {
		t.Fatalf("action = %+v", action)
	}

	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRight})
	browser.cursor = 1
	action = pressBrowser(browser, tea.KeyMsg{Type: tea.KeyEnter})
	if !action.close || action.insert != "id" {
		t.Fatalf("column action = %+v", action)
	}
}

func TestBrowserFuzzyFilter(t *testing.T) {
	browser := openTestBrowser(t)
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "email" {
		pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(browser.rows) == 0 || browser.rows[0].name != "email" {
		t.Fatalf("filtered rows = %+v", browser.rows)
	}
	action := pressBrowser(browser, tea.KeyMsg{Type: tea.KeyEnter})
	if action.insert != "email" {
		t.Fatalf("action = %+v", action)
	}

	// A non-matching query shows the empty state.
	browser = openTestBrowser(t)
	pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "zzz" {
		pressBrowser(browser, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if !strings.Contains(ansi.Strip(browser.View()), "no matches") {
		t.Fatalf("view:\n%s", browser.View())
	}
}

func TestBrowserRefreshAndClose(t *testing.T) {
	browser := openTestBrowser(t)
	if action := pressBrowser(browser, tea.KeyMsg{Type: tea.KeyCtrlR}); !action.refresh {
		t.Fatal("ctrl+r did not request refresh")
	}
	if action := pressBrowser(browser, tea.KeyMsg{Type: tea.KeyEsc}); !action.close {
		t.Fatal("esc did not close")
	}
}

func TestBrowserScrolling(t *testing.T) {
	var tables []db.TableMeta
	for i := 0; i < 30; i++ {
		tables = append(tables, db.TableMeta{Name: "t" + string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	browser := newSchemaBrowser()
	browser.SetSize(80, 18)
	browser.Open(tables)

	for i := 0; i < 10; i++ {
		pressBrowser(browser, tea.KeyMsg{Type: tea.KeyDown})
	}
	if browser.offset == 0 {
		t.Fatalf("offset did not scroll: cursor=%d offset=%d height=%d", browser.cursor, browser.offset, browser.bodyHeight)
	}
	if view := browser.View(); strings.Count(view, "\n") > browser.bodyHeight+2 {
		t.Fatalf("view too tall:\n%s", view)
	}
}

func TestBrowserEmpty(t *testing.T) {
	browser := newSchemaBrowser()
	browser.Open(nil)
	if !strings.Contains(ansi.Strip(browser.View()), "no tables found") {
		t.Fatalf("view:\n%s", browser.View())
	}
}
