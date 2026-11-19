package query

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightLineNumbersAndStrings(t *testing.T) {
	if got := ansi.Strip(HighlightLine("VALUES (1, 2.5, 'x')")); got != "VALUES (1, 2.5, 'x')" {
		t.Fatalf("got %q", got)
	}
}

func TestHighlightLineCommentConsumesWholeLine(t *testing.T) {
	got := HighlightLine("SELECT 1 -- trailing text")
	if !strings.Contains(got, "-- trailing text") {
		t.Fatalf("comment truncated: %q", got)
	}
}
