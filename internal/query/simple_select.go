package query

import "strings"

// SimpleSelectTable reports whether sql is a single simple SELECT over one
// table (optionally schema-qualified and aliased) and returns the table name.
// Joins, GROUP BY/HAVING, UNION, DISTINCT, subqueries/functions, and multiple
// statements are rejected; WHERE/ORDER BY/LIMIT/OFFSET are allowed.
func SimpleSelectTable(sql string) (string, bool) {
	statements := SplitStatements(sql)
	if len(statements) != 1 {
		return "", false
	}
	tokens := tokenizePlain(statements[0])
	if len(tokens) == 0 || strings.ToLower(tokens[0]) != "select" {
		return "", false
	}

	disallowed := map[string]bool{
		"join": true, "inner": true, "left": true, "right": true, "full": true,
		"outer": true, "cross": true, "group": true, "having": true,
		"union": true, "intersect": true, "except": true, "distinct": true,
		"with": true,
	}
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if token == "(" {
			// Rejects function calls and subqueries.
			return "", false
		}
		if disallowed[lower] {
			return "", false
		}
	}

	fromIndex := -1
	fromCount := 0
	for i, token := range tokens {
		if strings.ToLower(token) == "from" {
			fromIndex = i
			fromCount++
		}
	}
	if fromCount != 1 || fromIndex+1 >= len(tokens) {
		return "", false
	}

	table := tokens[fromIndex+1]
	if !isIdentifierToken(table) || isKeywordToken(table) {
		return "", false
	}

	// A comma between FROM and the next clause means another table.
	for i := fromIndex + 2; i < len(tokens); i++ {
		switch strings.ToLower(tokens[i]) {
		case "where", "order", "limit", "offset":
			i = len(tokens)
			continue
		}
		if tokens[i] == "," {
			return "", false
		}
	}

	return lastSegment(table), true
}
