package query

import (
	"strings"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/schema"
)

func testIndex() *schema.Index {
	return schema.BuildIndex([]db.TableMeta{
		{
			Schema: "main", Name: "users", Kind: db.TableKindTable,
			Columns: []db.ColumnMeta{
				{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
				{Name: "email", Type: "TEXT"},
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
	})
}

func TestTokenSpan(t *testing.T) {
	text := "SELECT email FROM users"
	cursor := strings.Index(text, "email") + 3 // inside the token
	start, end, token := tokenSpan(text, cursor)
	if token != "email" || start != strings.Index(text, "email") || end != start+5 {
		t.Fatalf("span = %d..%d %q", start, end, token)
	}

	start, end, token = tokenSpan("SELECT * FROM ", 14)
	if token != "" || start != 14 || end != 14 {
		t.Fatalf("empty span = %d..%d %q", start, end, token)
	}
}

func TestInStringOrComment(t *testing.T) {
	if !inStringOrComment("SELECT 'ema", 11) {
		t.Fatal("string not detected")
	}
	if inStringOrComment("SELECT email", 12) {
		t.Fatal("false positive outside string")
	}
	if !inStringOrComment("SELECT 1 -- note", 15) {
		t.Fatal("line comment not detected")
	}
	if !inStringOrComment("/* note */ SELECT", 5) {
		t.Fatal("block comment not detected")
	}
}

func TestAnalyzeContext(t *testing.T) {
	tests := []struct {
		prefix string
		clause string
		tables []string
	}{
		{"SELECT * FROM ", "from", nil},
		{"SELECT * FROM users u JOIN orders o ON ", "on", []string{"users", "orders"}},
		{"SELECT * FROM users WHERE ", "where", []string{"users"}},
		{"UPDATE users SET ", "set", []string{"users"}},
		{"INSERT INTO orders ", "insert into", []string{"orders"}},
		{"DELETE FROM orders WHERE ", "where", []string{"orders"}},
		{"SELECT * FROM users GROUP BY ", "group by", []string{"users"}},
		{"SELECT * FROM users ORDER BY ", "order by", []string{"users"}},
		{"SELECT * FROM users, orders WHERE ", "where", []string{"users", "orders"}},
		{"SELECT * FROM users AS u WHERE ", "where", []string{"users"}},
	}
	for _, tc := range tests {
		clause, refs := analyzeContext(tc.prefix)
		if clause != tc.clause {
			t.Fatalf("%q: clause = %q, want %q", tc.prefix, clause, tc.clause)
		}
		if len(refs) != len(tc.tables) {
			t.Fatalf("%q: refs = %+v, want %v", tc.prefix, refs, tc.tables)
		}
		for i, table := range tc.tables {
			if refs[i].Table != table {
				t.Fatalf("%q: refs = %+v", tc.prefix, refs)
			}
		}
	}
}

func TestSuggestionsFromTables(t *testing.T) {
	index := testIndex()
	text := "SELECT * FROM us"
	start := strings.Index(text, "us")
	items := suggestionsFor(index, text, start, len(text), nil)
	if len(items) == 0 || items[0].Name != "users" || items[0].Kind != kindTable {
		t.Fatalf("items = %+v", items)
	}
}

func TestSuggestionsAfterSelectUseAllColumns(t *testing.T) {
	index := testIndex()
	text := "SELECT em"
	start := strings.Index(text, "em")
	items := suggestionsFor(index, text, start, len(text), nil)
	if len(items) == 0 || items[0].Name != "email" || items[0].Kind != kindColumn {
		t.Fatalf("items = %+v", items)
	}
}

func TestSuggestionsScopedToFromTables(t *testing.T) {
	index := testIndex()
	text := "SELECT * FROM users WHERE em"
	start := strings.Index(text, "em")
	items := suggestionsFor(index, text, start, len(text), nil)
	if len(items) == 0 || items[0].Name != "email" || items[0].Table != "users" {
		t.Fatalf("items = %+v", items)
	}
}

func TestSuggestionsAfterJoinUseBothTables(t *testing.T) {
	index := testIndex()
	text := "SELECT * FROM users u JOIN orders o ON user"
	start := strings.LastIndex(text, "user")
	items := suggestionsFor(index, text, start, len(text), nil)
	if len(items) == 0 || items[0].Name != "user_id" {
		t.Fatalf("items = %+v", items)
	}
}

func TestSuggestionsForAliasQualifier(t *testing.T) {
	index := testIndex()
	text := "SELECT u. FROM users u"
	cursor := strings.Index(text, "u.") + 2
	start, end, _ := tokenSpan(text, cursor)
	items := suggestionsFor(index, text, start, end, nil)
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	for _, item := range items {
		if item.Kind != kindColumn || item.Table != "u" {
			t.Fatalf("item = %+v", item)
		}
	}
}

func TestSuggestionsKeywordsAndSchema(t *testing.T) {
	index := testIndex()
	text := "SEL"
	items := suggestionsFor(index, text, 0, 3, nil)
	if len(items) == 0 || items[0].Name != "SELECT" || items[0].Kind != kindKeyword {
		t.Fatalf("items = %+v", items)
	}

	text = "m"
	items = suggestionsFor(index, text, 0, 1, nil)
	foundSchema := false
	for _, item := range items {
		if item.Kind == kindSchema && item.Name == "main" {
			foundSchema = true
		}
	}
	if !foundSchema {
		t.Fatalf("schema suggestion missing: %+v", items)
	}
}

func TestSuggestionsSuppressedInsideString(t *testing.T) {
	index := testIndex()
	text := "SELECT 'ema"
	cursor := len(text)
	start, end, _ := tokenSpan(text, cursor)
	if items := suggestionsFor(index, text, start, end, nil); len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestRankingPrefersExactAndUsage(t *testing.T) {
	candidates := []popupItem{
		{Name: "email", Kind: kindColumn, Table: "users"},
		{Name: "id", Kind: kindColumn, Table: "users"},
		{Name: "user_id", Kind: kindColumn, Table: "orders"},
	}
	items := rankCandidates(candidates, "id", map[string]int{"user_id": 5})
	if len(items) == 0 || items[0].Name != "id" {
		t.Fatalf("items = %+v", items)
	}

	items = rankCandidates([]popupItem{
		{Name: "b", Kind: kindTable},
		{Name: "a", Kind: kindTable},
	}, "", map[string]int{"a": 3})
	if len(items) != 2 || items[0].Name != "a" {
		t.Fatalf("usage ranking = %+v", items)
	}
}

func TestRankingLimitsToTen(t *testing.T) {
	var candidates []popupItem
	for i := 0; i < 25; i++ {
		candidates = append(candidates, popupItem{Name: "t" + strings.Repeat("x", i%3) + string(rune('a'+i%26)), Kind: kindTable})
	}
	items := rankCandidates(candidates, "", nil)
	if len(items) != 10 {
		t.Fatalf("items = %d", len(items))
	}
}
