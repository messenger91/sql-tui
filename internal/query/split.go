package query

import "strings"

// SplitStatements splits SQL on top-level semicolons. Semicolons inside
// strings, quoted identifiers, comments, and PostgreSQL dollar-quoted blocks
// are preserved. Statements that contain only whitespace/comments are dropped.
//
// This is intentionally not a full SQL parser; it is the shared lightweight
// scanner that §10's autocomplete also builds on.
func SplitStatements(sql string) []string {
	statements := split(sql)
	out := make([]string, 0, len(statements))
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" || !hasCode(statement) {
			continue
		}
		out = append(out, statement)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func split(sql string) []string {
	var statements []string
	var current strings.Builder

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\'' || r == '"' || r == '`':
			// Quoted string or identifier; doubled quotes and backslash
			// escapes are consumed inside the literal.
			quote := r
			current.WriteRune(r)
			for i++; i < len(runes); i++ {
				current.WriteRune(runes[i])
				if runes[i] == '\\' && quote != '`' && i+1 < len(runes) {
					i++
					current.WriteRune(runes[i])
					continue
				}
				if runes[i] == quote {
					if i+1 < len(runes) && runes[i+1] == quote {
						i++
						current.WriteRune(runes[i])
						continue
					}
					break
				}
			}

		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				current.WriteRune(runes[i])
				i++
			}
			if i < len(runes) {
				current.WriteRune(runes[i])
			}

		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			current.WriteString("/*")
			i += 2
			for ; i < len(runes); i++ {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					current.WriteString("*/")
					i++
					break
				}
				current.WriteRune(runes[i])
			}

		case r == '$':
			if delimiter, next, ok := dollarQuote(runes, i); ok {
				current.WriteString(delimiter)
				i = next
				for ; i < len(runes); i++ {
					if strings.HasPrefix(string(runes[i:]), delimiter) {
						current.WriteString(delimiter)
						i += len(delimiter) - 1
						break
					}
					current.WriteRune(runes[i])
				}
				continue
			}
			current.WriteRune(r)

		case r == ';':
			statements = append(statements, current.String())
			current.Reset()

		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		statements = append(statements, current.String())
	}
	return statements
}

// dollarQuote recognises $$...$$ and $tag$...$tag$ openers. It returns the
// delimiter, the index just past the opener, and whether the opener is valid.
// Parameters like $1 are not dollar quotes.
func dollarQuote(runes []rune, start int) (string, int, bool) {
	i := start + 1
	if i >= len(runes) {
		return "", 0, false
	}
	if runes[i] == '$' {
		return "$$", i + 1, true
	}
	if !isTagStart(runes[i]) {
		return "", 0, false
	}
	for i < len(runes) && isTagPart(runes[i]) {
		i++
	}
	if i >= len(runes) || runes[i] != '$' {
		return "", 0, false
	}
	return string(runes[start : i+1]), i + 1, true
}

func isTagStart(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isTagPart(r rune) bool {
	return isTagStart(r) || r >= '0' && r <= '9'
}

// hasCode reports whether a statement contains anything besides whitespace
// and comments.
func hasCode(statement string) bool {
	runes := []rune(statement)
	for i := 0; i < len(runes); i++ {
		switch {
		case runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' || runes[i] == '\r':
			continue
		case runes[i] == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		case runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for ; i < len(runes); i++ {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					i++
					break
				}
			}
			continue
		default:
			return true
		}
	}
	return false
}
