package favorites

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

type item struct {
	favorite Favorite
}

func (i item) Title() string       { return i.favorite.Name }
func (i item) Description() string { return preview(i.favorite.SQL) }
func (i item) FilterValue() string { return strings.ToLower(i.favorite.Name) }

// preview renders the first meaningful line of the SQL, capped for the list.
func preview(sql string) string {
	line := strings.Join(strings.Fields(sql), " ")
	if len(line) > 60 {
		return line[:59] + "…"
	}
	return line
}

func items(favorites []Favorite) []list.Item {
	entries := make([]list.Item, 0, len(favorites))
	for _, favorite := range favorites {
		entries = append(entries, item{favorite: favorite})
	}
	return entries
}
