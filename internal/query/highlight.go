package query

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	hlKeyword   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	hlString    = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	hlNumber    = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	hlComment   = lipgloss.NewStyle().Faint(true)
	hlPunct     = lipgloss.NewStyle().Faint(true)
	hlIdent     = lipgloss.NewStyle()
	hlPlacehold = lipgloss.NewStyle().Faint(true)
)

var highlightKeywords = func() map[string]bool {
	set := make(map[string]bool)
	for _, keyword := range sqlKeywords {
		for _, word := range strings.Fields(keyword) {
			set[strings.ToLower(word)] = true
		}
	}
	return set
}()

// HighlightLine applies lightweight SQL syntax colours to one line.
func HighlightLine(line string) string {
	runes := []rune(line)
	var b strings.Builder
	i := 0
	write := func(text string, style lipgloss.Style) { b.WriteString(style.Render(text)) }

	for i < len(runes) {
		r := runes[i]
		switch {
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			end := i
			for end < len(runes) && runes[end] != '\n' {
				end++
			}
			write(string(runes[i:end]), hlComment)
			i = end

		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			end := i + 2
			for end+1 < len(runes) && !(runes[end] == '*' && runes[end+1] == '/') {
				end++
			}
			end = min(end+2, len(runes))
			write(string(runes[i:end]), hlComment)
			i = end

		case r == '\'' || r == '"' || r == '`':
			quote := r
			end := i + 1
			for end < len(runes) {
				if runes[end] == '\\' && quote != '`' {
					end += 2
					continue
				}
				if runes[end] == quote {
					if end+1 < len(runes) && runes[end+1] == quote {
						end += 2
						continue
					}
					end++
					break
				}
				end++
			}
			write(string(runes[i:end]), hlString)
			i = end

		case r >= '0' && r <= '9':
			end := i
			for end < len(runes) && ((runes[end] >= '0' && runes[end] <= '9') || runes[end] == '.') {
				end++
			}
			write(string(runes[i:end]), hlNumber)
			i = end

		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			end := i
			for end < len(runes) && isTokenRune(runes[end]) && runes[end] != '.' {
				end++
			}
			word := strings.ToLower(string(runes[i:end]))
			switch {
			case highlightKeywords[word]:
				write(string(runes[i:end]), hlKeyword)
			case end < len(runes) && runes[end] == '(':
				write(string(runes[i:end]), hlNumber)
			default:
				write(string(runes[i:end]), hlIdent)
			}
			i = end

		default:
			write(string(runes[i]), hlPunct)
			i++
		}
	}
	return b.String()
}
