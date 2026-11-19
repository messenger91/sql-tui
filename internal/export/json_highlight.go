package export

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	jsonKey   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	jsonStr   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	jsonNum   = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	jsonBool  = lipgloss.NewStyle().Foreground(lipgloss.Color("176"))
	jsonPunct = lipgloss.NewStyle().Faint(true)
)

// HighlightJSON adds lightweight syntax colours to a JSON document.
func HighlightJSON(text string) string {
	runes := []rune(text)
	var b strings.Builder
	i := 0
	write := func(s string, style lipgloss.Style) { b.WriteString(style.Render(s)) }

	for i < len(runes) {
		r := runes[i]
		switch {
		case r == '"':
			end := i + 1
			for end < len(runes) {
				if runes[end] == '\\' {
					end += 2
					continue
				}
				if runes[end] == '"' {
					end++
					break
				}
				end++
			}
			token := string(runes[i:end])
			if isJSONKey(runes, end) {
				write(token, jsonKey)
			} else {
				write(token, jsonStr)
			}
			i = end

		case r == '-' || r >= '0' && r <= '9':
			end := i + 1
			for end < len(runes) && ((runes[end] >= '0' && runes[end] <= '9') || runes[end] == '.' || runes[end] == 'e' || runes[end] == 'E' || runes[end] == '+' || runes[end] == '-') {
				end++
			}
			write(string(runes[i:end]), jsonNum)
			i = end

		case r >= 'a' && r <= 'z':
			end := i
			for end < len(runes) && runes[end] >= 'a' && runes[end] <= 'z' {
				end++
			}
			word := string(runes[i:end])
			switch word {
			case "true", "false", "null":
				write(word, jsonBool)
			default:
				write(word, jsonPunct)
			}
			i = end

		default:
			write(string(r), jsonPunct)
			i++
		}
	}
	return b.String()
}

func isJSONKey(runes []rune, after int) bool {
	for i := after; i < len(runes); i++ {
		switch runes[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}
