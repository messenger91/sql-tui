package query

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/schema"
)

type completionKind string

const (
	kindTable   completionKind = "table"
	kindView    completionKind = "view"
	kindColumn  completionKind = "column"
	kindKeyword completionKind = "keyword"
	kindSchema  completionKind = "schema"
)

type popupItem struct {
	Name  string
	Kind  completionKind
	Table string
	Type  string
}

type popupState struct {
	items  []popupItem
	cursor int
	start  int // rune span replaced on acceptance
	end    int
}

var sqlKeywords = []string{
	"SELECT", "FROM", "WHERE", "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "OUTER",
	"ON", "GROUP BY", "ORDER BY", "HAVING", "LIMIT", "OFFSET", "INSERT", "INTO",
	"VALUES", "UPDATE", "SET", "DELETE", "CREATE", "TABLE", "VIEW", "INDEX",
	"DROP", "ALTER", "AS", "AND", "OR", "NOT", "NULL", "IS", "IN", "EXISTS",
	"LIKE", "BETWEEN", "DISTINCT", "UNION", "ALL", "CASE", "WHEN", "THEN",
	"ELSE", "END", "ASC", "DESC", "PRIMARY", "KEY", "FOREIGN", "REFERENCES",
}

type tableRef struct {
	Table string
	Alias string
}

// tokenSpan returns the rune span of the identifier under the cursor.
func tokenSpan(text string, cursor int) (int, int, string) {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	start := cursor
	for start > 0 && isTokenRune(runes[start-1]) {
		start--
	}
	end := cursor
	for end < len(runes) && isTokenRune(runes[end]) {
		end++
	}
	return start, end, string(runes[start:end])
}

func isTokenRune(r rune) bool {
	return r == '_' || r == '$' || r == '.' ||
		r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func isIdentifierToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if !isTokenRune(r) {
			return false
		}
	}
	return true
}

// inStringOrComment reports whether pos falls inside a quoted string or
// comment, where completion should stay quiet.
func inStringOrComment(text string, pos int) bool {
	runes := []rune(text)
	if pos > len(runes) {
		pos = len(runes)
	}
	state := byte(0) // 0 normal, '\'' '"' '`' '-' line comment, '/' block comment
	for i := 0; i < pos; i++ {
		r := runes[i]
		switch state {
		case 0:
			switch {
			case r == '\'' || r == '"' || r == '`':
				state = byte(r)
			case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
				state = '-'
				i++
			case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
				state = '/'
				i++
			}
		case '-':
			if r == '\n' {
				state = 0
			}
		case '/':
			if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				state = 0
				i++
			}
		default:
			quote := rune(state)
			if r == '\\' && quote != '`' {
				i++
				continue
			}
			if r == quote {
				if i+1 < len(runes) && runes[i+1] == quote {
					i++
					continue
				}
				state = 0
			}
		}
	}
	return state != 0
}

// tokenizePlain returns the words and punctuation of the current statement,
// dropping strings and comments.
func tokenizePlain(text string) []string {
	var tokens []string
	runes := []rune(text)
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\'' || r == '"' || r == '`':
			flush()
			quote := r
			for i++; i < len(runes); i++ {
				if runes[i] == '\\' && quote != '`' {
					i++
					continue
				}
				if runes[i] == quote {
					if i+1 < len(runes) && runes[i+1] == quote {
						i++
						continue
					}
					break
				}
			}
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			flush()
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			flush()
			for i += 2; i < len(runes); i++ {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					i++
					break
				}
			}
		case r == ';':
			flush()
			tokens = tokens[:0]
		case r == ',' || r == '(' || r == ')':
			flush()
			tokens = append(tokens, string(r))
		case isTokenRune(r):
			word.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func nextToken(tokens []string, i int) string {
	if i+1 < len(tokens) {
		return strings.ToLower(tokens[i+1])
	}
	return ""
}

func isKeywordToken(token string) bool {
	switch strings.ToLower(token) {
	case "select", "from", "where", "join", "inner", "left", "right", "full",
		"outer", "on", "group", "by", "order", "having", "limit", "offset",
		"insert", "into", "values", "update", "set", "delete", "create", "table",
		"view", "index", "drop", "alter", "as", "and", "or", "not", "null",
		"is", "in", "exists", "like", "between", "distinct", "union", "all",
		"case", "when", "then", "else", "end", "asc", "desc", "primary", "key",
		"foreign", "references":
		return true
	}
	return false
}

// analyzeContext finds the current clause and the table references seen in the
// text before the cursor.
func analyzeContext(prefix string) (string, []tableRef) {
	tokens := tokenizePlain(prefix)
	clause := ""
	var refs []tableRef
	seen := make(map[string]bool)

	for i := 0; i < len(tokens); i++ {
		token := strings.ToLower(tokens[i])
		switch token {
		case "select", "where", "on", "having", "set", "values":
			clause = token
		case "group":
			if nextToken(tokens, i) == "by" {
				clause = "group by"
				i++
			}
		case "order":
			if nextToken(tokens, i) == "by" {
				clause = "order by"
				i++
			}
		case "from", "join", "update":
			clause = token
			i = parseTableRefs(tokens, i, &refs, seen)
		case "insert":
			if nextToken(tokens, i) == "into" {
				clause = "insert into"
				i = parseTableRefs(tokens, i+1, &refs, seen)
			}
		case "delete":
			if nextToken(tokens, i) == "from" {
				clause = "delete from"
				i = parseTableRefs(tokens, i+1, &refs, seen)
			}
		}
	}
	return clause, refs
}

func parseTableRefs(tokens []string, keyword int, refs *[]tableRef, seen map[string]bool) int {
	j := keyword + 1
	for j < len(tokens) {
		token := tokens[j]
		if token == "," {
			j++
			continue
		}
		if !isIdentifierToken(token) || isKeywordToken(token) {
			return j - 1
		}
		table := lastSegment(token)
		alias := table
		j++
		if j < len(tokens) && strings.EqualFold(tokens[j], "as") {
			j++
			if j < len(tokens) {
				alias = tokens[j]
				j++
			}
		} else if j < len(tokens) && isIdentifierToken(tokens[j]) && !isKeywordToken(tokens[j]) {
			alias = tokens[j]
			j++
		}
		key := strings.ToLower(alias)
		if !seen[key] {
			seen[key] = true
			*refs = append(*refs, tableRef{Table: table, Alias: alias})
		}
		if j < len(tokens) && tokens[j] == "," {
			j++
			continue
		}
		return j - 1
	}
	return j - 1
}

func lastSegment(name string) string {
	if position := strings.LastIndex(name, "."); position >= 0 {
		return name[position+1:]
	}
	return name
}

// suggestionsFor computes ranked completion candidates for the token span.
func suggestionsFor(index *schema.Index, text string, start, end int, usage map[string]int) []popupItem {
	if index == nil || start == end {
		return nil
	}
	if inStringOrComment(text, start) {
		return nil
	}

	token := string([]rune(text)[start:end])
	qualifier := ""
	partial := token
	if dot := strings.LastIndex(token, "."); dot >= 0 {
		qualifier = token[:dot]
		partial = token[dot+1:]
	}

	prefix := string([]rune(text)[:start])
	clause, refs := analyzeContext(prefix)
	_, allRefs := analyzeContext(text)

	candidates := candidatesFor(index, clause, qualifier, refs, allRefs)
	return rankCandidates(candidates, partial, usage)
}

func candidatesFor(index *schema.Index, clause, qualifier string, refs, allRefs []tableRef) []popupItem {
	tables := index.Tables()
	if qualifier != "" {
		tableName := qualifier
		for _, ref := range allRefs {
			if strings.EqualFold(ref.Alias, qualifier) {
				tableName = ref.Table
				break
			}
		}
		table, ok := index.Table(tableName)
		if !ok {
			return nil
		}
		return columnItems(table, qualifier)
	}

	switch clause {
	case "from", "join", "update", "insert into", "delete from":
		items := make([]popupItem, 0, len(tables))
		for _, table := range tables {
			kind := kindTable
			if table.Kind == db.TableKindView {
				kind = kindView
			}
			items = append(items, popupItem{Name: table.Name, Kind: kind})
		}
		return items
	case "select", "where", "on", "having", "set", "values", "group by", "order by":
		var items []popupItem
		if len(refs) == 0 {
			for _, table := range tables {
				items = append(items, columnItems(table, table.Name)...)
			}
			return items
		}
		for _, ref := range refs {
			if table, ok := index.Table(ref.Table); ok {
				items = append(items, columnItems(table, ref.Alias)...)
			}
		}
		return items
	default:
		items := make([]popupItem, 0, len(sqlKeywords)+1)
		if len(tables) > 0 && tables[0].Schema != "" {
			items = append(items, popupItem{Name: tables[0].Schema, Kind: kindSchema})
		}
		for _, keyword := range sqlKeywords {
			items = append(items, popupItem{Name: keyword, Kind: kindKeyword})
		}
		return items
	}
}

func columnItems(table db.TableMeta, owner string) []popupItem {
	items := make([]popupItem, 0, len(table.Columns))
	for _, column := range table.Columns {
		items = append(items, popupItem{
			Name:  column.Name,
			Kind:  kindColumn,
			Table: owner,
			Type:  column.Type,
		})
	}
	return items
}

func kindRank(kind completionKind) int {
	switch kind {
	case kindTable:
		return 0
	case kindView:
		return 1
	case kindColumn:
		return 2
	case kindSchema:
		return 3
	default:
		return 4
	}
}

// matchesFuzzy reports whether pattern's runes appear in order in target.
func matchesFuzzy(pattern, target string) bool {
	if pattern == "" {
		return true
	}
	return len(fuzzy.Find(pattern, []string{target})) > 0
}

func rankCandidates(candidates []popupItem, partial string, usage map[string]int) []popupItem {
	type scored struct {
		item  popupItem
		score int
	}
	lowerPartial := strings.ToLower(partial)
	var matches []scored
	for _, candidate := range candidates {
		name := strings.ToLower(candidate.Name)
		score := -1
		switch {
		case partial == "":
			score = 1
		case name == lowerPartial:
			score = 0
		case strings.HasPrefix(name, lowerPartial):
			score = 1
		case matchesFuzzy(lowerPartial, name):
			score = 2
		}
		if score < 0 {
			continue
		}
		matches = append(matches, scored{item: candidate, score: score})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		if left.score != right.score {
			return left.score < right.score
		}
		if kindRank(left.item.Kind) != kindRank(right.item.Kind) {
			return kindRank(left.item.Kind) < kindRank(right.item.Kind)
		}
		leftUsage := usage[strings.ToLower(left.item.Name)]
		rightUsage := usage[strings.ToLower(right.item.Name)]
		if leftUsage != rightUsage {
			return leftUsage > rightUsage
		}
		return strings.ToLower(left.item.Name) < strings.ToLower(right.item.Name)
	})

	items := make([]popupItem, 0, min(len(matches), 10))
	seen := make(map[string]bool)
	for _, match := range matches {
		key := strings.ToLower(string(match.item.Kind) + ":" + match.item.Name + ":" + match.item.Table)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, match.item)
		if len(items) == 10 {
			break
		}
	}
	return items
}
