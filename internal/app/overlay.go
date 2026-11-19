package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlay splices foreground onto background at the horizontal centre of
// width, and vertically centred. Both strings may contain ANSI escapes; the
// splice is cell-aware so styled backgrounds are not corrupted.
func overlay(background, foreground string, width int) string {
	bgLines := strings.Split(background, "\n")
	fgLines := strings.Split(foreground, "\n")
	if len(fgLines) == 0 || width <= 0 {
		return background
	}

	fgWidth := ansi.StringWidth(fgLines[0])
	x := max((width-fgWidth)/2, 0)
	y := max((len(bgLines)-len(fgLines))/2, 0)

	for i, fg := range fgLines {
		line := y + i
		if line >= len(bgLines) {
			break
		}
		bgLines[line] = spliceLine(bgLines[line], fg, x)
	}
	return strings.Join(bgLines, "\n")
}

func spliceLine(background, foreground string, x int) string {
	left := ansi.Truncate(background, x, "")
	if pad := x - ansi.StringWidth(left); pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	right := ansi.TruncateLeft(background, x+ansi.StringWidth(foreground), "")
	return left + foreground + right
}
