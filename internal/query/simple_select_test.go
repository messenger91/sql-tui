package query

import "testing"

func TestSimpleSelectTable(t *testing.T) {
	tests := []struct {
		name  string
		sql   string
		table string
		ok    bool
	}{
		{"simple", "SELECT * FROM users", "users", true},
		{"columns", "SELECT id, email FROM users", "users", true},
		{"where", "SELECT id FROM users WHERE id > 1", "users", true},
		{"order limit", "SELECT id FROM users ORDER BY id DESC LIMIT 10", "users", true},
		{"offset", "SELECT id FROM users LIMIT 10 OFFSET 5", "users", true},
		{"alias", "SELECT u.id FROM users u", "users", true},
		{"as alias", "SELECT u.id FROM users AS u", "users", true},
		{"qualified", "SELECT id FROM public.users", "users", true},
		{"quoted string", "SELECT id FROM users WHERE email = 'a,b'", "users", true},
		{"join", "SELECT * FROM users u JOIN orders o ON o.user_id = u.id", "", false},
		{"left join", "SELECT * FROM users LEFT JOIN orders ON 1=1", "", false},
		{"group by", "SELECT count(*) FROM users GROUP BY id", "", false},
		{"having", "SELECT id FROM users HAVING id > 1", "", false},
		{"union", "SELECT id FROM users UNION SELECT id FROM orders", "", false},
		{"distinct", "SELECT DISTINCT id FROM users", "", false},
		{"subquery", "SELECT id FROM (SELECT id FROM users) t", "", false},
		{"function", "SELECT lower(email) FROM users", "", false},
		{"two tables", "SELECT * FROM users, orders", "", false},
		{"no from", "SELECT 1", "", false},
		{"two statements", "SELECT * FROM users; SELECT 1", "", false},
		{"insert", "UPDATE users SET id = 1", "", false},
		{"comment", "-- note\nSELECT id FROM users", "users", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			table, ok := SimpleSelectTable(tc.sql)
			if ok != tc.ok || table != tc.table {
				t.Fatalf("SimpleSelectTable(%q) = %q, %v; want %q, %v", tc.sql, table, ok, tc.table, tc.ok)
			}
		})
	}
}
