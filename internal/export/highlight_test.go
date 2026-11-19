package export

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightJSONPreservesText(t *testing.T) {
	input := "{\n  \"id\": 1,\n  \"name\": \"ada\",\n  \"admin\": true,\n  \"note\": null\n}"
	if got := ansi.Strip(HighlightJSON(input)); got != input {
		t.Fatalf("highlight = %q", got)
	}
}

func TestHighlightJSONKeysAndValues(t *testing.T) {
	out := HighlightJSON(`{"name": "ada", "n": 1}`)
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, `"ada"`) {
		t.Fatalf("highlight = %q", out)
	}
}
