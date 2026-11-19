package query

import (
	"reflect"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{"single", "SELECT 1", []string{"SELECT 1"}},
		{"trailing semicolon", "SELECT 1;", []string{"SELECT 1"}},
		{"two statements", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"semicolon in string", "SELECT ';' AS semi", []string{"SELECT ';' AS semi"}},
		{"doubled quote", "SELECT 'it''s;' AS v", []string{"SELECT 'it''s;' AS v"}},
		{"line comment", "SELECT 1 -- ;\n; SELECT 2", []string{"SELECT 1 -- ;", "SELECT 2"}},
		{"block comment", "/* ; */ SELECT 1", []string{"/* ; */ SELECT 1"}},
		{"dollar quote", "SELECT $$a;b$$; SELECT 2", []string{"SELECT $$a;b$$", "SELECT 2"}},
		{"tagged dollar quote", "SELECT $tag$a;b$tag$ AS x; SELECT 2", []string{"SELECT $tag$a;b$tag$ AS x", "SELECT 2"}},
		{"bind parameter is not a quote", "SELECT $1, $2", []string{"SELECT $1, $2"}},
		{"comments only", "  ; -- nothing\n ; ", nil},
		{"mysql backslash escape", `SELECT 'a\';b'; SELECT 2`, []string{`SELECT 'a\';b'`, "SELECT 2"}},
		{"leading whitespace", "\n\tSELECT 1  \n", []string{"SELECT 1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitStatements(tc.sql)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SplitStatements(%q) = %#v, want %#v", tc.sql, got, tc.want)
			}
		})
	}
}
