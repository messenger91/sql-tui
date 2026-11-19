package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Explain runs the driver's query-plan statement and renders it as text.
// statement must be a single SQL statement.
func Explain(ctx context.Context, handle *sql.DB, driver Driver, statement string, maxRows int) (string, error) {
	statement = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(statement), ";"))
	if statement == "" {
		return "", fmt.Errorf("db: explain: empty statement")
	}
	var wrapped string
	switch driver {
	case DriverPostgres:
		wrapped = "EXPLAIN (FORMAT TEXT) " + statement
	case DriverMySQL:
		wrapped = "EXPLAIN FORMAT=TREE " + statement
	case DriverSQLite:
		wrapped = "EXPLAIN QUERY PLAN " + statement
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedDriver, driver)
	}

	result := Execute(ctx, handle, wrapped, maxRows)
	if result.Err != nil {
		return "", fmt.Errorf("db: explain: %w", result.Err)
	}
	return FormatPlan(driver, result), nil
}

// FormatPlan renders an executed plan. SQLite's tabular EXPLAIN QUERY PLAN is
// converted into an indented tree; text plans are returned line by line.
func FormatPlan(driver Driver, result QueryResult) string {
	if len(result.Columns) == 0 {
		return ""
	}
	if driver == DriverSQLite {
		if plan, ok := formatSQLitePlan(result); ok {
			return plan
		}
	}
	var lines []string
	for _, row := range result.Rows {
		parts := make([]string, 0, len(row))
		for _, value := range row {
			parts = append(parts, planValue(value))
		}
		lines = append(lines, strings.Join(parts, " | "))
	}
	return strings.Join(lines, "\n")
}

type sqlitePlanNode struct {
	id     int64
	parent int64
	detail string
}

func formatSQLitePlan(result QueryResult) (string, bool) {
	idIndex, parentIndex, detailIndex := -1, -1, -1
	for i, column := range result.Columns {
		switch strings.ToLower(column) {
		case "id":
			idIndex = i
		case "parent":
			parentIndex = i
		case "detail":
			detailIndex = i
		}
	}
	if idIndex < 0 || parentIndex < 0 || detailIndex < 0 {
		return "", false
	}

	nodes := make(map[int64]sqlitePlanNode, len(result.Rows))
	for _, row := range result.Rows {
		if max(idIndex, parentIndex, detailIndex) >= len(row) {
			continue
		}
		id, ok := planInt(row[idIndex])
		if !ok {
			continue
		}
		parent, _ := planInt(row[parentIndex])
		nodes[id] = sqlitePlanNode{id: id, parent: parent, detail: planValue(row[detailIndex])}
	}
	if len(nodes) == 0 {
		return "", false
	}

	children := make(map[int64][]int64)
	for id, node := range nodes {
		if node.parent == id {
			continue
		}
		children[node.parent] = append(children[node.parent], id)
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool { return children[parent][i] < children[parent][j] })
	}

	var lines []string
	var walk func(id int64, depth int)
	visited := make(map[int64]bool)
	walk = func(id int64, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true
		node := nodes[id]
		lines = append(lines, strings.Repeat("  ", depth)+node.detail)
		for _, child := range children[id] {
			walk(child, depth+1)
		}
	}

	var roots []int64
	for id, node := range nodes {
		if _, ok := nodes[node.parent]; !ok || node.parent == 0 {
			roots = append(roots, id)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	for _, root := range roots {
		walk(root, 0)
	}
	// Defensive: include any cyclic leftovers.
	for id := range nodes {
		walk(id, 0)
	}
	return strings.Join(lines, "\n"), true
}

func planInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case []byte:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func planValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
