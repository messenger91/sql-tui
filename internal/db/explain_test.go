package db

import (
	"context"
	"strings"
	"testing"
)

func TestExplainSQLiteTree(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	mustExec(t, handle, `INSERT INTO users (id, email) VALUES (1, 'a')`)

	plan, err := Explain(context.Background(), handle, DriverSQLite, "SELECT * FROM users WHERE id = 1", 0)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if plan == "" {
		t.Fatal("empty plan")
	}
	lower := strings.ToLower(plan)
	if !strings.Contains(lower, "search") && !strings.Contains(lower, "scan") {
		t.Fatalf("plan = %q", plan)
	}
}

func TestFormatPlanSQLiteTree(t *testing.T) {
	result := QueryResult{
		Columns: []string{"id", "parent", "notused", "detail"},
		Rows: [][]any{
			{int64(0), int64(0), int64(0), "SCAN a"},
			{int64(1), int64(0), int64(0), "SEARCH b USING INDEX"},
		},
	}
	plan := FormatPlan(DriverSQLite, result)
	if !strings.Contains(plan, "SCAN a\n  SEARCH b") {
		t.Fatalf("plan = %q", plan)
	}
}

func TestFormatPlanText(t *testing.T) {
	result := QueryResult{
		Columns: []string{"QUERY PLAN"},
		Rows:    [][]any{{[]byte("Seq Scan on users")}, {[]byte("  Filter: id = 1")}},
	}
	plan := FormatPlan(DriverPostgres, result)
	if plan != "Seq Scan on users\n  Filter: id = 1" {
		t.Fatalf("plan = %q", plan)
	}
}

func TestExplainUnsupportedDriver(t *testing.T) {
	if _, err := Explain(context.Background(), nil, Driver("oracle"), "SELECT 1", 0); err == nil {
		t.Fatal("unsupported driver accepted")
	}
}
