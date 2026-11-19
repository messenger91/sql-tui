package query

import (
	"testing"
)

func TestAnalyzeContextEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		clause string
		tables []string
	}{
		{
			name:   "escaped quotes",
			prefix: "SELECT 'it''s' FROM users WHERE ",
			clause: "where",
			tables: []string{"users"},
		},
		{
			name:   "line comment before clause",
			prefix: "SELECT * -- FROM ignored\nFROM users WHERE ",
			clause: "where",
			tables: []string{"users"},
		},
		{
			name:   "block comment before clause",
			prefix: "SELECT /* FROM ignored */ * FROM users WHERE ",
			clause: "where",
			tables: []string{"users"},
		},
		{
			name:   "multiple statements use the last one",
			prefix: "SELECT * FROM orders; SELECT * FROM users WHERE ",
			clause: "where",
			tables: []string{"users"},
		},
		{
			name:   "trailing whitespace",
			prefix: "SELECT * FROM users   ",
			clause: "from",
			tables: []string{"users"},
		},
		{
			name:   "alias with AS",
			prefix: "SELECT * FROM orders AS o JOIN users AS u ON ",
			clause: "on",
			tables: []string{"orders", "users"},
		},
		{
			name:   "schema-qualified table",
			prefix: "SELECT * FROM public.users WHERE ",
			clause: "where",
			tables: []string{"users"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clause, refs := analyzeContext(tc.prefix)
			if clause != tc.clause {
				t.Fatalf("clause = %q, want %q", clause, tc.clause)
			}
			if len(refs) != len(tc.tables) {
				t.Fatalf("refs = %+v, want %v", refs, tc.tables)
			}
			for i := range tc.tables {
				if refs[i].Table != tc.tables[i] {
					t.Fatalf("refs = %+v, want %v", refs, tc.tables)
				}
			}
		})
	}
}

func TestSubqueryScopeIsBestEffort(t *testing.T) {
	// Subqueries are documented as best-effort: the parser does not descend
	// into them, so a WHERE outside the subquery falls back to all columns.
	index := testIndex()
	text := "SELECT * FROM (SELECT id FROM users) u WHERE em"
	start := len(text) - 2
	items := suggestionsFor(index, text, start, len(text), nil)
	if len(items) == 0 {
		t.Fatal("expected fallback column suggestions")
	}
	if items[0].Name != "email" {
		t.Fatalf("items = %+v", items)
	}
}

func TestRankingCaseInsensitivePrefixes(t *testing.T) {
	items := rankCandidates([]popupItem{
		{Name: "users", Kind: kindTable},
		{Name: "user_id", Kind: kindColumn},
		{Name: "orders", Kind: kindTable},
	}, "US", nil)
	if len(items) < 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Name != "users" {
		t.Fatalf("table with exact prefix should come first: %+v", items)
	}
	if items[1].Name != "user_id" {
		t.Fatalf("prefix matches beat fuzzy: %+v", items)
	}
}

func TestRankingFuzzyTier(t *testing.T) {
	items := rankCandidates([]popupItem{
		{Name: "users", Kind: kindTable},
		{Name: "orders", Kind: kindTable},
	}, "usr", nil)
	if len(items) != 1 || items[0].Name != "users" {
		t.Fatalf("items = %+v", items)
	}
}
